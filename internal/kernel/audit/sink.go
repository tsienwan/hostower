package audit

import (
	"encoding/json"
	"os"
	"sync"
)

// FileSink 把每条记录追加写成一行 JSON，作为数据库之外的副本。
// 把它指到另一块盘或由日志采集器收走，塔台被攻破也删不干净。
type FileSink struct {
	mu sync.Mutex
	f  *os.File
}

// NewFileSink 以只追加方式打开文件。
func NewFileSink(path string) (*FileSink, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &FileSink{f: f}, nil
}

func (s *FileSink) Write(r Record) error {
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.f.Write(append(line, '\n'))
	return err
}

func (s *FileSink) Close() error { return s.f.Close() }
