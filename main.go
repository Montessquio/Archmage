package main

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// UNIX Time is faster and smaller than most timestamps
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	log.Info().Msg("Strike the Earth!")

	// Read the Token out of the secrets folder. This is TODO
	token, err := os.ReadFile("./secret/token")
	if err != nil {
		log.Fatal().Str("Err", err.Error()).Msg("Could not read token! ")
	}

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + string(token))
	if err != nil {
		log.Fatal().Msg(fmt.Sprintf("Error creating Discord session: %s", err.Error()))
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(ProcessMessage)

	// In this example, we only care about receiving message events.
	dg.Identify.Intents = discordgo.IntentsGuildMessages

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		log.Fatal().Msg(fmt.Sprintf("Error opening connection: %s", err.Error()))
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	log.Info().Msg("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

// Precompiled regex for speed and safety.
var argRegex *regexp.Regexp = regexp.MustCompile(`[^\s"']+|("[^"]*")|('[^']*')`)
var diceExprRegex *regexp.Regexp = regexp.MustCompile(`^\(*\d*d\d+`)

// Recieves a discord message, determines if it is a command,
// processes its arguments and runs the associated command function.
// Commands are entirely self contained and should not fail outwards.
// Commands not found are simply ignored.
func ProcessMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Silently do nothing if this message is not a command.
	if !isCommand(m.Content) {
		return
	}

	args := toArgs(stripPrefix(m.Content))

	// Handle `help` command separately, since golang can't
	// resolve having help refer to the struct its callback is stored in.
	if args[0] == "help" {
		HelpHandler(s, m, args)
		return
	}

	// Handle the special ![diceexpr] condition.
	if diceExprRegex.MatchString(args[0]) {
		RollHandler(s, m, args)
		return
	}

	callback := lookupCommand(args[0])
	if callback == nil {
		return // Ignore non-registered commands.
	}

	callback(s, m, args)
}

// Find the command out of the commands dictionary.
// Takes the command name as an argument and
// returns a command callback or nil if the function is not found
func lookupCommand(name string) func(*discordgo.Session, *discordgo.MessageCreate, []string) {
	if cmd, ok := CommandMap[name]; ok {
		return cmd.Callback
	} else {
		return nil
	}
}

// Tests whether a message begins with the command prefix.
func isCommand(s string) bool {
	return strings.HasPrefix(s, "!")
}

// Removes whatever the command prefix is from the message.
func stripPrefix(s string) string {
	_, i := utf8.DecodeRuneInString(s)
	return s[i:]
}

// Processes a message string into a command and list of arguments.
// Assumes that the command prefix is removed, but does not assume a valid command.
func toArgs(content string) []string {
	return argRegex.FindAllString(content, -1)
}
