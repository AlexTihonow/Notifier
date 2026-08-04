package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	EmailHost     string
	EmailPort     int
	EmailSMTPAuth bool
	EmailUsername string
	EmailPassword string
	EmailFrom     string

	RudnAPIKey      string
	RudnClientLogin string

	DbUser 			string
	DbAddress 		string
	DbName 			string
	DbPass			string
}

func LoadConfig() (Config, error) {

	v := viper.New()

	v.AutomaticEnv()

	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	_ = v.ReadInConfig()

	v.SetDefault("EMAIL_PORT", "465")
	v.SetDefault("EMAIL_SMTPAUTH", true)


	cfg := Config{
		EmailHost:     v.GetString("EMAIL_HOST"),
		EmailPort:     v.GetInt("EMAIL_PORT"),
		EmailSMTPAuth: v.GetBool("EMAIL_SMTPAUTH"),
		EmailUsername: v.GetString("EMAIL_USERNAME"),
		EmailPassword: v.GetString("EMAIL_PASSWORD"),
		EmailFrom:     v.GetString("EMAIL_FROM"),

		RudnAPIKey:      v.GetString("X_API_KEY"),
		RudnClientLogin: v.GetString("X_CLIENT_LOGIN"),

		DbUser:      	v.GetString("DB_USER"),
		DbAddress: 		v.GetString("DB_ADDRESS"),
		DbName: 		v.GetString("DB_NAME"),
		DbPass: 		v.GetString("DB_PASSWORD"),
	}

	return cfg, nil
}
