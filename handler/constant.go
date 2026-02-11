package handler

import (
	"database/sql"
	"sync"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
)

// Конфигурация сервера
type ServerConfig struct {
	GuildID          string `json:"server_id" db:"guild_id"`
	RegistrationRole string `json:"registration_role_id"`
	CategoryID       string `json:"category_id"`
	CommandChannelID string `json:"command_channel_id"`
	GuildRoleId      string `json:"guild_role_id"`
	FriendRoleId     string `json:"friend_role_id"`
}

// RegistrationConfig - основная структура конфигурации
type RegistrationConfig struct {
	Version    string     `json:"version"`
	Questions  []Question `json:"questions"`
	Completion Completion `json:"completion"`
}

// Question - вопрос регистрации
type Question struct {
	ID         string     `json:"id"`
	Order      int        `json:"order"`
	Type       string     `json:"type"` // single_choice, multiple_choice, text_input, number_input
	Required   bool       `json:"required"`
	Text       string     `json:"text"`
	Options    []Option   `json:"options,omitempty"`
	Validation *Validation `json:"validation,omitempty"`
	Actions    []Action   `json:"actions,omitempty"`
	Next       NextStep   `json:"next"`
}

// Option - вариант ответа
type Option struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	RoleID string `json:"role_id,omitempty"`
}

// Validation - правила валидации
type Validation struct {
	MinLength int    `json:"min_length,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
	Regex     string `json:"regex,omitempty"`
	MinValue  int    `json:"min_value,omitempty"`
	MaxValue  int    `json:"max_value,omitempty"`
}

// Action - действие при ответе
type Action struct {
	Type    string                 `json:"type"`
	RoleID  string                 `json:"role_id,omitempty"`
	Field   string                 `json:"field,omitempty"`
	Storage string                 `json:"storage,omitempty"` // session, permanent
	Value   string                 `json:"value,omitempty"`   // "@selected.id", "@selected.role_id", "@input"
	Format  string                 `json:"format,omitempty"`
	Config  map[string]interface{} `json:"config,omitempty"` // Для дополнительных параметров
}

// NextStep - определение следующего шага
type NextStep struct {
	Type       string      `json:"type"` // static, conditional, end
	QuestionID string      `json:"question_id,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	Default    string      `json:"default,omitempty"`
}

// Condition - условие перехода
type Condition struct {
	If        ConditionCheck `json:"if"`
	QuestionID string        `json:"question_id"`
}

// ConditionCheck - проверка условия
type ConditionCheck struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // equals, not_equals, contains, greater, less
	Value    interface{} `json:"value"`
}

// Completion - действия при завершении
type Completion struct {
	Message string   `json:"message"`
	Actions []Action `json:"actions,omitempty"`
}

// Ответ пользователя
type UserAnswer struct {
	QuestionID string      `json:"question_id"`
	Value      interface{} `json:"value"` // string, []string, int, etc.
	Selected   *Option     `json:"selected,omitempty"` // Для choice типов
}

// Сессия пользователя
type UserSession struct {
	UserID      string                 `json:"user_id"`
	ChannelID   string                 `json:"channel_id"`
	CurrentQID  string                 `json:"current_question_id"`
	Answers     map[string]UserAnswer  `json:"answers"`
	Data        map[string]interface{} `json:"data"` // session storage
	StartedAt   int64                  `json:"started_at"`
}

// BotHandler - основной обработчик бота
type BotHandler struct {
	session *discordgo.Session
}

// NewBotHandler - создание нового обработчика бота
func NewBotHandler(session *discordgo.Session) *BotHandler {
	return &BotHandler{session: session}
}

// Глобальные переменные
var (
	DBPath           = "./registration.db"
	db               *sql.DB
	registrationConfigs = make(map[string]*RegistrationConfig) // guild_id -> config
	serverConfigs    = make(map[string]*ServerConfig)           // guild_id -> config
	registeringUsers = make(map[string]*UserSession)
	mu               sync.Mutex
)

// ForEachServerConfig - функция для перебора всех зарегистрированных серверов
func ForEachServerConfig(fn func(guildID string, config *ServerConfig)) {
	mu.Lock()
	defer mu.Unlock()
	for guildID, config := range serverConfigs {
		fn(guildID, config)
	}
}

var logger = SetupLogger()
