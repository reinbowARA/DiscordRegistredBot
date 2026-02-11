package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Проверка прав администратора
func IsAdmin(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	perms, err := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
	if err != nil {
		logger.Error("Ошибка проверки прав: " + err.Error())
		return false
	}
	return perms&discordgo.PermissionAdministrator != 0
}

// Поиск ID роли по имени
func findRoleID(s *discordgo.Session, guildID, roleID string) string {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		logger.Error("Ошибка получения ролей: " + err.Error())
		return ""
	}

	for _, role := range roles {
		if strings.EqualFold(role.ID, roleID) {
			return role.ID
		}
	}
	return ""
}

// Инициализация базы данных
func InitDB() error {
	var err error
	db, err = sql.Open("sqlite3", DBPath)
	if err != nil {
		return err
	}

	// Создание таблицы
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS registration_configs(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id TEXT NOT NULL UNIQUE,
		meta_data TEXT NOT NULL CHECK(json_valid(meta_data)),
		config_json TEXT NOT NULL CHECK(json_valid(config_json)),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		return err
	}

	// Загрузка конфигураций в память
	return LoadConfigsFromDB()
}

// Загрузка конфигураций из базы данных
func LoadConfigsFromDB() error {
	rows, err := db.Query("SELECT guild_id, meta_data, config_json FROM registration_configs")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var guildID, metaDataStr, configJSONStr string
		err := rows.Scan(&guildID, &metaDataStr, &configJSONStr)
		if err != nil {
			return err
		}

		// Парсим ServerConfig из meta_data
		var serverConfig ServerConfig
		if err := json.Unmarshal([]byte(metaDataStr), &serverConfig); err != nil {
			logger.Error("Ошибка парсинга ServerConfig для гильдии " + guildID + ": " + err.Error())
			continue
		}
		serverConfigs[guildID] = &serverConfig

		// Парсим RegistrationConfig из config_json
		var regConfig RegistrationConfig
		if err := json.Unmarshal([]byte(configJSONStr), &regConfig); err != nil {
			logger.Error("Ошибка парсинга RegistrationConfig для гильдии " + guildID + ": " + err.Error())
			continue
		}
		registrationConfigs[guildID] = &regConfig
	}

	return rows.Err()
}

// Сохранение конфигурации в базу данных
func SaveConfigToDB(guildID string, serverConfig *ServerConfig, regConfig *RegistrationConfig) error {
	metaDataJSON, err := json.Marshal(serverConfig)
	if err != nil {
		return err
	}

	configJSON, err := json.Marshal(regConfig)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT OR REPLACE INTO registration_configs (guild_id, meta_data, config_json, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		guildID, string(metaDataJSON), string(configJSON))
	return err
}

// Получение конфигурации для гильдии
func GetServerConfig(guildID string) (*ServerConfig, bool) {
	mu.Lock()
	defer mu.Unlock()
	config, exists := serverConfigs[guildID]
	return config, exists
}

// Получение RegistrationConfig для гильдии
func GetRegistrationConfig(guildID string) (*RegistrationConfig, bool) {
	mu.Lock()
	defer mu.Unlock()
	config, exists := registrationConfigs[guildID]
	return config, exists
}

// Загрузка вопросов из JSON (для обратной совместимости, но теперь из БД)
func LoadQuestions() error {
	// Инициализируем БД
	return InitDB()
}

func SetupLogger() *slog.Logger {

	opts := &slog.HandlerOptions{
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("timestamp", time.Now().Format("2006-01-02 15:04:05"))
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	}

	writers := []io.Writer{os.Stdout}

	if logDir := os.Getenv("LOG_DIR"); logDir != "" {
		writers = append(writers, &lumberjack.Logger{
			Filename:   filepath.Join(logDir, "bot.log"),
			MaxSize:    100, // MB
			MaxBackups: 14,  // файлов
			MaxAge:     30,  // дней
			Compress:   true,
		})
	}

	multiWriter := io.MultiWriter(writers...)

	baseHandler := slog.NewJSONHandler(multiWriter, opts)

	filtredHandler := &levelFilterHandler{handler: baseHandler}

	return slog.New(filtredHandler)
}

type levelFilterHandler struct {
	handler slog.Handler
}

func (h *levelFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level == slog.LevelInfo || level == slog.LevelError
}

func (h *levelFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.Enabled(ctx, r.Level) {
		return h.handler.Handle(ctx, r)
	}
	return nil
}

func (h *levelFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelFilterHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *levelFilterHandler) WithGroup(name string) slog.Handler {
	return &levelFilterHandler{handler: h.handler.WithGroup(name)}
}
