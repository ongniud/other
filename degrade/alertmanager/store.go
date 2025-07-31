package alertmanager

import (
	"encoding/json"
	"fmt"
	"github.com/prometheus/prometheus/model/labels"
	"os"
	"path/filepath"
)

// Storage 定义持久化存储接口
type Storage interface {
	SaveAlerts(r *Rule, alerts []IAlert) error
	LoadAlerts(r *Rule) ([]IAlert, error)
}

// FileStorage 实现文件系统存储
type FileStorage struct {
	path string
}

func NewFileStorage(path string) (*FileStorage, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %v", err)
	}
	return &FileStorage{path: path}, nil
}

func (fs *FileStorage) SaveAlerts(r *Rule, alerts []IAlert) error {
	var raw [][]byte
	for _, alert := range alerts {
		data, err := alert.Marshal()
		if err != nil {
			return fmt.Errorf("failed to marshal alert: %v", err)
		}
		raw = append(raw, data)
	}

	combined, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal alert list: %v", err)
	}

	filename := filepath.Join(fs.path, fmt.Sprintf("%s.json", r.Name))
	tmpFilename := filename + ".tmp"
	if err := os.WriteFile(tmpFilename, combined, 0644); err != nil {
		return fmt.Errorf("failed to write alerts to temp file: %w", err)
	}
	if err := os.Rename(tmpFilename, filename); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

func (fs *FileStorage) LoadAlerts(r *Rule) ([]IAlert, error) {
	filename := filepath.Join(fs.path, fmt.Sprintf("%s.json", r.Name))
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read alert file: %v", err)
	}

	var rawList [][]byte
	if err := json.Unmarshal(data, &rawList); err != nil {
		return nil, fmt.Errorf("failed to unmarshal alert list: %v", err)
	}

	var alerts []IAlert
	for _, raw := range rawList {
		alert, err := NewAlert(r.AlertType, labels.EmptyLabels(), r.AlertOpts)
		if err != nil {
			return nil, err
		}
		if err := alert.Restore(raw, r.AlertOpts); err != nil {
			return nil, fmt.Errorf("failed to restore alert: %v", err)
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

type MemoryStorage struct {
	alerts map[string][][]byte
}

func (m *MemoryStorage) SaveAlerts(r *Rule, alerts []IAlert) error {
	var raw [][]byte
	for _, alert := range alerts {
		data, err := alert.Marshal()
		if err != nil {
			return err
		}
		raw = append(raw, data)
	}
	m.alerts[r.Name] = raw
	return nil
}

func (m *MemoryStorage) LoadAlerts(r *Rule) ([]IAlert, error) {
	raw, exists := m.alerts[r.Name]
	if !exists {
		return nil, nil
	}

	var alerts []IAlert
	for _, data := range raw {
		alert, err := NewAlert(r.AlertType, labels.EmptyLabels(), r.AlertOpts)
		if err != nil {
			return nil, err
		}
		if err := alert.Restore(data, r.AlertOpts); err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}
