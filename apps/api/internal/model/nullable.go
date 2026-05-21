package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// NullString 是 sql.NullString 的薄壳,只为了让 JSON 序列化:
// Valid=false → null;Valid=true → "字符串"。
// GORM Scan/Value 走嵌入的 sql.NullString,完全兼容。
type NullString struct {
	sql.NullString
}

func StringFrom(value string) NullString {
	return NullString{sql.NullString{String: value, Valid: value != ""}}
}

func (n NullString) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.String)
}

func (n *NullString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		n.Valid = false
		n.String = ""
		return nil
	}
	if err := json.Unmarshal(data, &n.String); err != nil {
		return err
	}
	n.Valid = true
	return nil
}

// NullTime 同理。Valid=false → null;Valid=true → RFC3339 字符串。
type NullTime struct {
	sql.NullTime
}

func TimeFrom(value time.Time) NullTime {
	return NullTime{sql.NullTime{Time: value, Valid: !value.IsZero()}}
}

func (n NullTime) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Time)
}

func (n *NullTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		n.Valid = false
		return nil
	}
	if err := json.Unmarshal(data, &n.Time); err != nil {
		return err
	}
	n.Valid = true
	return nil
}
