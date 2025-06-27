package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/reinbowARA/DiscordRegistredBot/handler"
)

var Logger = handler.SetupLogger()

func main() {
	var srv handler.ServerConfig
	if err := handler.LoadQuestions(); err != nil {
		Logger.Error("Ошибка загрузки вопросов: "+err.Error())
		return
	}
	session, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		Logger.Error("Ошибка создания сессии: "+err.Error())
		return
	}

	session.AddHandler(srv.NewGuildMember)
	session.AddHandler(srv.MessageCreate)

	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuilds

	err = session.Open()
	if err != nil {
		Logger.Error("Ошибка подключения: "+err.Error())
		return
	}
	defer session.Close()

	Logger.Info("Бот запущен! Для остановки Ctrl+C")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
