// Help command handler
package main

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Requires special handling due to Golang's inability to cyclically include help in its own command list.
func HelpHandler(s *discordgo.Session, m *discordgo.MessageCreate, _ []string) {
	var commandNames strings.Builder
	var commandShorts strings.Builder

	for name, cmd := range CommandMap {
		commandNames.WriteString(name)
		commandNames.WriteString("\n\n")

		commandShorts.WriteString(cmd.Short)
		commandShorts.WriteString("\n\n")
	}

	// Manually add help to list due to above reason.
	commandNames.WriteString("help\n\n")
	commandShorts.WriteString("Displays this list.\n\n")

	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{},
		Color:       0x00ff00, // Green
		Description: "Here's a list of everything you can do with me! Try `!help [command]` for more details and usage examples.",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Command",
				Value:  commandNames.String(),
				Inline: true,
			},
			{
				Name:   "Description",
				Value:  commandShorts.String(),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339), // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
		Title:     "Archmage: Command List",
	}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}
