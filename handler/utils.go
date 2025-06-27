package handler

import (
	"context"
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

// Загрузка вопросов из JSON
func LoadQuestions() error {
	file, err := os.ReadFile(QuestionsFilePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(file, &questions)
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
