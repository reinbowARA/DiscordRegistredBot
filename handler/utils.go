package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Проверка прав администратора
func IsAdmin(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	perms, err := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
	if err != nil {
		fmt.Println("Ошибка проверки прав:", err)
		return false
	}
	return perms&discordgo.PermissionAdministrator != 0
}

// Поиск ID роли по имени
func findRoleID(s *discordgo.Session, guildID, roleID string) string {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		fmt.Println("Ошибка получения ролей:", err)
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
