// The !ping command replies with Pong and a millisecond count describing the time between the incoming message timestamp and the time-of-sending.
package main

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

func PingHandler(s *discordgo.Session, m *discordgo.MessageCreate, _ []string) {
	msgTime, err := m.Timestamp.Parse()
	if err != nil {
		log.Error().Str("Err", err.Error()).Msg("Discord timestamp parsing failed!")
	}
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Pong! (%dms)", time.Now().Sub(msgTime).Milliseconds()))
}
