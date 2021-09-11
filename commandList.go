// A dictionary of all commands and their associated callbacks.
package main

import (
	"github.com/bwmarrin/discordgo"
)

var CommandMap = map[string]Command{
	"ping": {
		Name:     "ping",
		Short:    "Test bot responsiveness",
		Desc:     "Responds with Pong! and the time it took the bot to respond.",
		Example:  "!ping",
		Callback: PingHandler,
	},
	"roll": {
		Name:     "roll",
		Short:    "Roll a dice expression.",
		Desc:     "Roll any dice or mathematical expression. The \"roll\" prefix is unnecessary as long as the first term is some sort of die expression.",
		Example:  "!roll [expr], !roll 3d6+(1d4/2), !3d6+(1d4/2)",
		Callback: RollHandler,
	},
}

type Command struct {
	Name     string
	Short    string
	Desc     string
	Example  string
	Callback func(*discordgo.Session, *discordgo.MessageCreate, []string)
}
