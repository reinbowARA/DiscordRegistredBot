package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/reinbowARA/DiscordRegistredBot/handler"
)

func main() {
	var srv handler.ServerConfig

	if err := handler.LoadQuestions(); err != nil {
		fmt.Println("Ошибка загрузки вопросов:", err)
		return
	}
	session, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		fmt.Println("Ошибка создания сессии:", err)
		return
	}

	session.AddHandler(srv.NewGuildMember)
	session.AddHandler(srv.MessageCreate)

	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuilds

	err = session.Open()
	if err != nil {
		fmt.Println("Ошибка подключения:", err)
		return
	}
	defer session.Close()

	fmt.Println("Бот запущен! Для остановки Ctrl+C")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
