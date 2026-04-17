package config

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Server ServerCfg `toml:"server"`
	Log    LogCfg    `toml:"log"`
	Mongo  MongoCfg  `toml:"mongo"`
	Redis  RedisCfg  `toml:"redis"`
	JWT    JWTCfg    `toml:"jwt"`
	WS     WSCfg     `toml:"ws"`
	APNs   APNsCfg   `toml:"apns"`
	CDN    CDNCfg    `toml:"cdn"`
	Hash   string    `toml:"-"`
}

type ServerCfg struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
	TLS  bool   `toml:"tls"`
}

type LogCfg struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Output string `toml:"output"`
}

type MongoCfg struct {
	URI        string `toml:"uri"`
	DB         string `toml:"db"`
	TimeoutSec int    `toml:"timeout_sec"`
}

type RedisCfg struct {
	Addr string `toml:"addr"`
	DB   int    `toml:"db"`
}

type JWTCfg struct {
	PrivateKeyPath    string `toml:"private_key_path"`
	PrivateKeyPathOld string `toml:"private_key_path_old"`
	ActiveKID         string `toml:"active_kid"`
	OldKID            string `toml:"old_kid"`
	Issuer            string `toml:"issuer"`
	AccessExpirySec   int    `toml:"access_expiry_sec"`
	RefreshExpirySec  int    `toml:"refresh_expiry_sec"`
}

type WSCfg struct {
	MaxConnections int `toml:"max_connections"`
}

type APNsCfg struct {
	KeyID    string `toml:"key_id"`
	TeamID   string `toml:"team_id"`
	BundleID string `toml:"bundle_id"`
	KeyPath  string `toml:"key_path"`
}

type CDNCfg struct {
	BaseURL string `toml:"base_url"`
}

func MustLoad(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal().Err(err).Str("path", path).Msg("config read failed")
	}

	h := sha256.Sum256(data)
	hash := fmt.Sprintf("%x", h[:4])

	var c Config
	if _, err := toml.Decode(string(data), &c); err != nil {
		log.Fatal().Err(err).Str("path", path).Msg("config decode failed")
	}

	c.Hash = hash
	c.mustValidate()
	return &c
}

func (c *Config) mustValidate() {
	if c.Server.Port < 0 {
		log.Fatal().Int("port", c.Server.Port).Msg("config: server.port must be >= 0")
	}
}
