package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/reinbowARA/DiscordRegistredBot/handler"
)

var Logger = handler.SetupLogger()

// Проверка, является ли сервер зарегистрированным
func isRegisteredGuild(guildID string) bool {
	if guildID == "" {
		return false
	}
	
	// Проверяем наличие конфигурации сервера
	_, exists := handler.GetServerConfig(guildID)
	return exists
}

// Обработчик новых участников
func guildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	// Проверяем, зарегистрирован ли сервер
	if !isRegisteredGuild(m.GuildID) {
		// Игнорируем события от незарегистрированных серверов
		return
	}

	// Получаем конфигурацию сервера для конкретной гильдии
	serverConfig, exists := handler.GetServerConfig(m.GuildID)
	if !exists {
		Logger.Warn("Конфигурация сервера не найдена для зарегистрированной гильдии " + m.GuildID)
		return
	}

	// Вызываем метод на правильной конфигурации
	serverConfig.NewGuildMember(s, m)
}

// Обработчик сообщений
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Игнорируем сообщения от ботов
	if m.Author.Bot {
		return
	}

	// Получаем GuildID из контекста сообщения
	var guildID string
	if m.GuildID != "" {
		guildID = m.GuildID
	} else {
		// Если GuildID не доступен напрямую, попробуем получить его через канал
		channel, err := s.Channel(m.ChannelID)
		if err == nil && channel.GuildID != "" {
			guildID = channel.GuildID
		}
	}

	// Обрабатываем команду !init даже для незарегистрированных серверов
	if strings.HasPrefix(m.Content, "!init") {
		// Создаем временную конфигурацию для обработки команды init
		tempConfig := &handler.ServerConfig{GuildID: guildID}
		tempConfig.MessageCreate(s, m)
		return
	}

	// Проверяем, зарегистрирован ли сервер для остальных команд
	if !isRegisteredGuild(guildID) {
		// Игнорируем сообщения от незарегистрированных серверов
		return
	}

	// Получаем конфигурацию сервера
	serverConfig, exists := handler.GetServerConfig(guildID)
	if !exists {
		Logger.Warn("Конфигурация сервера не найдена для зарегистрированной гильдии " + guildID)
		return
	}

	// Вызываем метод на правильной конфигурации
	serverConfig.MessageCreate(s, m)
}

func main() {
	godotenv.Load()
	if err := handler.LoadQuestions(); err != nil {
		Logger.Error("Ошибка загрузки вопросов: " + err.Error())
		return
	}

	// Проверяем, есть ли зарегистрированные серверы
	registeredCount := 0
	handler.ForEachServerConfig(func(guildID string, config *handler.ServerConfig) {
		registeredCount++
		Logger.Info("Зарегистрирован сервер: " + guildID)
	})

	if registeredCount == 0 {
		Logger.Info("Нет зарегистрированных серверов. Используйте команду !init load_server для загрузки конфигурации.")
	} else {
		Logger.Info("Загружено конфигураций для " + string(rune(registeredCount)) + " серверов")
	}

	session, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		Logger.Error("Ошибка создания сессии: " + err.Error())
		return
	}

	session.AddHandler(guildMemberAdd)
	session.AddHandler(messageCreate)

	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuilds

	err = session.Open()
	if err != nil {
		Logger.Error("Ошибка подключения: " + err.Error())
		return
	}
	defer session.Close()

	Logger.Info("Бот запущен! Для остановки Ctrl+C")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
