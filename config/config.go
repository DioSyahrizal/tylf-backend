package config

import (
	"log"

	"github.com/spf13/viper"
)

type Env struct {
	DB_HOST              string `mapstructure:"DB_HOST"`
	DB_USER              string `mapstructure:"DB_USER"`
	DB_PASSWORD          string `mapstructure:"DB_PASSWORD"`
	DB_NAME              string `mapstructure:"DB_NAME"`
	DB_PORT              int    `mapstructure:"DB_PORT"`
	APP_ENV              string `mapstructure:"APP_ENV"`
	GOOGLE_CLIENT_ID     string `mapstructure:"GOOGLE_CLIENT_ID"`
	GOOGLE_CLIENT_SECRET string `mapstructure:"GOOGLE_CLIENT_SECRET"`
}

func Load() *Env {
	viper.SetConfigFile(".env")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Can't read .env file: %v", err)
	}

	var env Env
	if err := viper.Unmarshal(&env); err != nil {
		log.Fatalf("Failed to unmarshal env: %v", err)
	}

	return &env
}
