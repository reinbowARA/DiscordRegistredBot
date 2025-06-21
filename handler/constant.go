package handler

import "sync"

// Конфигурация сервера
type ServerConfig struct {
	GuildID          string `json:"server_id"`
	RegistrationRole string `json:"registration_role_id"`
	CategoryID       string `json:"category_id"`
	CommandChannelID string `json:"command_channel_id"`
	GuildRoleId      string `json:"guild_role_id"`
	FriendRoleId     string `json:"friend_role_id"`
}

// Глобальные переменные
var (
	configFile        = "./config/server_config.json"
	QuestionsFilePath = "./config/question.json"
	questions         []Question
	registeringUsers  = make(map[string]*RegistrationState)
	mu                sync.Mutex
)

// Структуры для вопросов
type Question struct {
	Question string            `json:"question"`
	Result   string            `json:"result"`
	Switch   map[string]string `json:"switch,omitempty"`
}

// Состояние регистрации
type RegistrationState struct {
	Step            int
	ChannelID       string
	Answers         []string
	CurrentQuestion *Question
}
