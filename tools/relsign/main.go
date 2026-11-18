// relsign 是发布用的签名工具。
//
//	go run ./tools/relsign gen             生成一对发布密钥（私钥只打印这一次）
//	go run ./tools/relsign seal <文件>...   用同一把私钥给可执行文件加构建封签（接在文件末尾；先 seal 再 sign）
//	go run ./tools/relsign sign <文件>...   用环境变量 HOSTOWER_RELEASE_KEY 里的私钥签名，写出 <文件>.sig
//	go run ./tools/relsign fingerprint <公钥>  打印公钥指纹（公布在 README 里，供用户对照）
//	go run ./tools/relsign verify <公钥> <文件>
//
// 公钥编译进可执行文件（-X main.updateKey=…），私钥只放在发布流程的机密里，不进仓库。
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/tsienwan/hostower/internal/update"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法：relsign gen | seal <文件>... | sign <文件>... | verify <公钥> <文件> | fingerprint <公钥>")
	}
	switch args[0] {
	case "gen":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		fmt.Println("公钥（编译进可执行文件，可以公开）：")
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		fmt.Println("私钥（放进发布流程的机密 HOSTOWER_RELEASE_KEY；丢了就再也发不了更新，泄露了别人就能发）：")
		fmt.Println(base64.StdEncoding.EncodeToString(priv.Seed()))
		fmt.Println("公钥指纹（写进 README 的“官方构建”一节）：")
		fmt.Println(update.Fingerprint(pub))
		return nil
	case "fingerprint":
		if len(args) != 2 {
			return fmt.Errorf("用法：relsign fingerprint <公钥>")
		}
		key, err := update.ParseKey(args[1])
		if err != nil {
			return err
		}
		fmt.Println(update.Fingerprint(key))
		return nil
	case "seal":
		priv, err := update.KeyFromSeed(os.Getenv("HOSTOWER_RELEASE_KEY"))
		if err != nil {
			return fmt.Errorf("环境变量 HOSTOWER_RELEASE_KEY 里没有合法的私钥")
		}
		for _, f := range args[1:] {
			bin, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			if err := os.WriteFile(f, update.Seal(priv, bin), 0o755); err != nil {
				return err
			}
			fmt.Println("已加封签", f)
		}
		return nil
	case "sign":
		seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("HOSTOWER_RELEASE_KEY")))
		if err != nil || len(seed) != ed25519.SeedSize {
			return fmt.Errorf("环境变量 HOSTOWER_RELEASE_KEY 里没有合法的私钥")
		}
		priv := ed25519.NewKeyFromSeed(seed)
		for _, f := range args[1:] {
			bin, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			if err := os.WriteFile(f+".sig", []byte(update.Sign(priv, bin)+"\n"), 0o644); err != nil {
				return err
			}
			fmt.Println("已签名", f)
		}
		return nil
	case "verify":
		if len(args) != 3 {
			return fmt.Errorf("用法：relsign verify <公钥> <文件>")
		}
		key, err := update.ParseKey(args[1])
		if err != nil {
			return err
		}
		bin, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(args[2] + ".sig")
		if err != nil {
			return err
		}
		if err := update.Verify(key, bin, sig); err != nil {
			return err
		}
		fmt.Println("签名正确")
		return nil
	}
	return fmt.Errorf("不认识的子命令 %q", args[0])
}
