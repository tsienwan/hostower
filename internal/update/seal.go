package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

// 构建封签：发布流程把“发布私钥对这个可执行文件的签名”接在文件末尾，程序启动时用编译进来的发布公钥校验自己。
// 校验通过说明：这个文件就是持有发布私钥的人签出来的那一份，一个字节都没改过。
//
// 它防不住的事要说清楚：别人改了源码、换成自己的公钥再编译，界面上同样会显示“校验通过”，只是公钥指纹不一样。
// 所以“是不是官方构建”最终要靠对照公钥指纹（官方指纹公布在 README 里），或者用 `hostower verify` 离线校验。
//
// 格式：原文件 || 64 字节签名 || 16 字节标记。签名覆盖的是带用途前缀的原文件摘要，和发布页上的 .sig 用的不是同一种消息，不能互相顶替。

const sealMagic = "HOSTOWER-SEAL-V1"

// sealLen 是接在文件末尾的封签长度。
const sealLen = ed25519.SignatureSize + len(sealMagic)

func sealMessage(digest []byte) []byte {
	return []byte("hostower-build-v1\n" + hex.EncodeToString(digest))
}

// Seal 给可执行文件加封签：返回接上了封签的新内容。已经封过的文件先去掉旧封签再封。
func Seal(priv ed25519.PrivateKey, bin []byte) []byte {
	if n := len(bin); n >= sealLen && string(bin[n-len(sealMagic):]) == sealMagic {
		bin = bin[:n-sealLen]
	}
	d := sha256.Sum256(bin)
	sig := ed25519.Sign(priv, sealMessage(d[:]))
	out := make([]byte, 0, len(bin)+sealLen)
	out = append(out, bin...)
	out = append(out, sig...)
	return append(out, sealMagic...)
}

// 构建的状态。
const (
	BuildOfficial = "signed"   // 封签用编译进来的发布公钥校验通过
	BuildUnsealed = "unsealed" // 没有封签：自己编译的，或者开发版
	BuildInvalid  = "invalid"  // 有封签但对不上：文件被改过，或者签名用的不是这把公钥
	BuildNoKey    = "nokey"    // 没有编译进发布公钥：无从校验
)

// BuildInfo 是“这个可执行文件是谁签出来的”的结论。
type BuildInfo struct {
	Status string `json:"status"`
	// Fingerprint 是编译进来的发布公钥的指纹，用来和官方公布的指纹对照。
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Fingerprint 返回公钥的指纹：公钥 SHA-256 的前 16 字节，四个字符一组。
func Fingerprint(key ed25519.PublicKey) string {
	d := sha256.Sum256(key)
	h := strings.ToUpper(hex.EncodeToString(d[:16]))
	var parts []string
	for i := 0; i < len(h); i += 4 {
		parts = append(parts, h[i:i+4])
	}
	return strings.Join(parts, "-")
}

// CheckSeal 校验一个文件的封签。pubKey 是 base64 的发布公钥；为空表示这个构建没有公钥。
func CheckSeal(path, pubKey string) BuildInfo {
	if strings.TrimSpace(pubKey) == "" {
		return BuildInfo{Status: BuildNoKey}
	}
	key, err := ParseKey(pubKey)
	if err != nil {
		return BuildInfo{Status: BuildNoKey}
	}
	info := BuildInfo{Fingerprint: Fingerprint(key)}
	f, err := os.Open(path)
	if err != nil {
		info.Status = BuildInvalid
		return info
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() < int64(sealLen) {
		info.Status = BuildUnsealed
		return info
	}
	tail := make([]byte, sealLen)
	if _, err := f.ReadAt(tail, st.Size()-int64(sealLen)); err != nil || !bytes.Equal(tail[ed25519.SignatureSize:], []byte(sealMagic)) {
		info.Status = BuildUnsealed
		return info
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(f, 0, st.Size()-int64(sealLen))); err != nil {
		info.Status = BuildInvalid
		return info
	}
	if ed25519.Verify(key, sealMessage(h.Sum(nil)), tail[:ed25519.SignatureSize]) {
		info.Status = BuildOfficial
	} else {
		info.Status = BuildInvalid
	}
	return info
}

// VerifyFile 是给命令行用的离线校验：用给定的公钥（base64）检查一个文件的封签。
func VerifyFile(path, pubKey string) (BuildInfo, error) {
	if _, err := ParseKey(pubKey); err != nil {
		return BuildInfo{}, err
	}
	info := CheckSeal(path, pubKey)
	switch info.Status {
	case BuildOfficial:
		return info, nil
	case BuildUnsealed:
		return info, errors.New("这个文件没有构建封签")
	default:
		return info, errors.New("封签对不上：文件被改过，或者不是用这把公钥签的")
	}
}

// KeyFromSeed 从 base64 的私钥种子得到私钥（发布工具用）。
func KeyFromSeed(s string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("私钥不合法")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
