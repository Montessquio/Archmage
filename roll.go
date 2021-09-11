// A dice parser and roller library.
package main

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

/******************
	COMMAND HANDLER
******************/

func RollHandler(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	// If they used !roll, remove that from the args list. Otherwise they used ![expr]
	if args[0] == "roll" {
		args = args[1:]
	}

	// Convert the input string into a token stream
	tokens, err := tokenizeExpr(strings.Join(args, ""))
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Error: %s", err.Error()))
		return
	}

	// Convert the token stream into a a syntax tree
	parser := NewDiceParser(tokens)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Error: %s", err.Error()))
	}

	// Assemble the AST
	expr := parser.Expr()
	if len(parser.errors) != 0 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Errs: %v\n", parser.errors))
	}

	// Walk and Resolve the AST
	result, work := expr.Eval()

	// Send a nice stylish message.
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{},
		Color:       0x00ff00, // Green
		Description: strings.Join(args, ""),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Rolls",
				Value:  work,
				Inline: false,
			},
			{
				Name:   "Result",
				Value:  strconv.Itoa(result),
				Inline: false,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339), // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
		Title:     m.Author.Username + "#" + m.Author.Discriminator + " Rolled " + strconv.Itoa(result),
	}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

/******************
	LEXER
******************/

func tokenizeExpr(raw string) ([]Token, error) {
	var tokens []Token

	var sb strings.Builder
	for _, char := range raw {
		// Consume until a transition token is reached
		switch char {
		case '\t', '\n', '\v', '\f', '\r', ' ', '\x85', '\xA0':
			continue // Ignore whitespace.
		case '+', '-', '*', '/', '(', ')':
			// The previous token is over.
			// Parse it before working on the current one.
			if sb.Len() != 0 {
				t, err := LexToken(sb.String())
				if err != nil {
					return nil, err
				}
				tokens = append(tokens, t)
			}

			// Now narrow down the token type to one of the three.
			var typ TokenType
			switch char {
			case '(', ')':
				typ = Group
			case '*', '/':
				typ = Factor
			case '+', '-':
				typ = Term
			default:
				panic("Unreachable!")
			}

			// Append the operator token to the queue.
			tokens = append(tokens, Token{typ, string(char)})

			// Clear the token buffer for the next batch.
			sb.Reset()
			continue
		default:
			// This is a non-transition token.
			sb.WriteRune(char)
		}
	}
	// Parse any remaining characters in the buffer
	// that may have not been terminated by an operator.
	if sb.Len() != 0 {
		t, err := LexToken(sb.String())
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

type Token struct {
	Type  TokenType
	Value string
}

type TokenType int

const (
	Const  TokenType = 0 // Number
	Die    TokenType = 1 // NdX i.e. 3d6
	Term   TokenType = 2 // +-
	Factor TokenType = 3 // */
	Group  TokenType = 4 // ()
)

// Precompiled regular expression for matching on die expressions.
var dieExpr *regexp.Regexp = regexp.MustCompile(`^\d*d\d+$`)

// Parses either a die or value expression from a string.
// Returns an Error if the token is not valid.
func LexToken(token string) (Token, error) {
	// Check for a Const Value Expr
	if isInt(token) {
		return Token{Type: Const, Value: token}, nil
	}

	// Check for a Die Value Expr.
	if dieExpr.MatchString(token) {
		if strings.HasPrefix(token, "d") {
			// If the left hand of the expression is empty,
			// that means it's an implied 1.
			token = "1" + token
		}

		return Token{Type: Die, Value: token}, nil
	}

	return Token{}, errors.New(fmt.Sprintf("\"%s\" was not recognized as a valid number or dice expression.", token))
}

// Helper function for ParseToken. Checks if a string is only numbers.
func isInt(s string) bool {
	for _, c := range s {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

/******************
	PARSER & AST
******************/

// A parser that converts a dice expression token stream to
// an AST and evaluates it according to the following grammar:
/*
	Expr	=> Term
	Term	=> Factor  ([ '+' | '-' ]) Factor)*
	Factor 	=> Primary ([ '*' | '/' ] Primary)*
	Primary => '(' Expr ')' | DIE | NUMBER
*/
type DiceParser struct {
	tokens  []Token
	current int
	errors  []error
}

func NewDiceParser(tokens []Token) DiceParser {
	return DiceParser{tokens, 0, make([]error, 0)}
}

// Satisfies the rule `Expr => Term`.
func (p *DiceParser) Expr() AstExpr {
	return p.Term()
}

// Satisfies the rule for `Term	=> Factor  ([ '+' | '-' ]) Factor)*`
func (p *DiceParser) Term() AstExpr {
	var expr AstExpr = p.Factor() // Left

	for p.check(Term) {
		t := p.consume()
		operator := t       // A Token
		right := p.Factor() // An AstExpr
		expr = AstOp{expr, right, operator}
	}

	return expr
}

// Satisfies the rule for `Factor 	=> Primary ([ '*' | '/' ] Primary)*`
func (p *DiceParser) Factor() AstExpr {
	expr := p.Primary()

	for p.check(Factor) {
		t := p.consume()
		operator := t        // A Token
		right := p.Primary() // An AstExpr
		expr = AstOp{expr, right, operator}
	}

	return expr
}

// Satisfies the rule for `Primary => '(' Expr ')' | DIE | NUMBER`
func (p *DiceParser) Primary() AstExpr {
	//log.Error().Str("Val", fmt.Sprintf("%v", p.peek())).Bool("Eq?", p.peek().Type == Const).Msg("Fuck")

	// If the current token is a Constant value..
	if p.check(Const) {
		t := p.consume()

		// This should never fail because the tokenizer verifies that
		// this kind of token is purely numeric.
		value, err := strconv.Atoi(t.Value)
		if err != nil {
			p.errors = append(p.errors, errors.New(fmt.Sprintf("Found a NUMBER token that was not purely numeric: '%s'", t.Value)))
			log.Error().Str("Value", t.Value).Str("Error", err.Error()).Msg("NUMBER token was not purely numeric! This should never happen!")
		}
		return AstConst(value)
	}

	if p.check(Die) {
		t := p.consume()

		splitDie := strings.Split(t.Value, "d")
		// A valid die expression is one with 2 parts, and the second part must be present and numeric.
		if (len(splitDie) != 2) || (!isInt(splitDie[1])) {
			p.errors = append(p.errors, errors.New(fmt.Sprintf("\"%s\" was not recognized as a valid number or dice expression.", t.Value)))
			return nil
		}

		// An empty first string indicates that the die is of the format `dXX`
		// in which case there is an implied preceding 1.
		if splitDie[0] == "" {
			splitDie[0] = "1"
		}

		// This should never fail because the tokenizer verifies that
		// this kind of token is purely numeric.
		left, err := strconv.Atoi(splitDie[0])
		if err != nil {
			p.errors = append(p.errors, errors.New(fmt.Sprintf("\"%s\" NUMBER in dice expression was not purely numeric.", t.Value)))
			log.Error().Str("Value", t.Value).Str("Error", err.Error()).Msg("NUMBER token was not purely numeric! This should never happen!")
		}

		right, err := strconv.Atoi(splitDie[1])
		if err != nil {
			p.errors = append(p.errors, errors.New(fmt.Sprintf("\"%s\" NUMBER in dice expression was not purely numeric.", t.Value)))
			log.Error().Str("Value", t.Value).Str("Error", err.Error()).Msg("NUMBER token was not purely numeric! This should never happen!")
		}
		return AstDie{left, right}
	}

	if p.check(Group) && p.peek().Value == "(" {
		p.consume()

		// In the case of a group, recurse back to the lowest priority and build a new subtree.
		expr := p.Expr()
		// Expect a closing paren.
		if p.check(Group) && p.peek().Value == ")" {
			p.consume()
			return expr
		} else {
			// Error, unmatched Paren.
			p.errors = append(p.errors, errors.New("Unmatched parenthesis."))
			return nil
		}
	}

	panic("Unreachable!")
}

// Consumes the current token if it matches the given type,
// advancing the cursor and returning it. Otherwise does nothing.
func (p *DiceParser) consume() Token {
	if !p.isAtEnd() {
		// Advance the cursor and return whatever was before it.
		p.current += 1
		return p.tokens[p.current-1]
	}
	// If we are at the end, then there's only one token left to consume.
	return p.tokens[p.current]
}

// Returns whether the current token is of the
// given type. Does not consume.
func (p DiceParser) check(t TokenType) bool {
	return p.peek().Type == t
}

// Get the current token without advancing nor consuming it.
func (p DiceParser) peek() Token {
	return p.tokens[p.current]
}

// Returns whether the `current` field is equal to
// the length of the token buf - 1
func (p DiceParser) isAtEnd() bool {
	return p.current == (len(p.tokens) - 1)
}

// An AST Expression is any object which can resolve itself
// to a final sum and a set of rolls (if any)
type AstExpr interface {
	// Eval returns a result and a "steps string"
	Eval() (int, string)
}

// A constant value's evaulation is just itself.
type AstConst int

func (c AstConst) Eval() (int, string) {
	return int(c), strconv.Itoa(int(c))
}

// A die's value is rolled, 1-[right] rolled [left] times, then summed.
type AstDie struct {
	left  int
	right int
}

func (t AstDie) Eval() (int, string) {
	var sb strings.Builder
	sb.WriteRune('[')

	rand.Seed(time.Now().UnixNano())
	rolls := make([]int, t.left)
	for i := range rolls {
		//out[i] = rand.Intn(max-min+1) + min
		rolls[i] = rand.Intn(int(t.right)) + 1
		sb.WriteString(strconv.Itoa(rolls[i]))
		if i != (len(rolls) - 1) {
			sb.WriteString(", ")
		}
	}
	sb.WriteRune(']')

	// Sum values
	sum := 0
	for _, v := range rolls {
		sum += v
	}
	return sum, sb.String()
}

type AstOp struct {
	Left  AstExpr
	Right AstExpr
	Op    Token
}

func (t AstOp) Eval() (int, string) {
	left, lwork := t.Left.Eval()
	right, rwork := t.Right.Eval()

	var sum int = 0
	var sb strings.Builder

	sb.WriteString(lwork)
	sb.WriteRune(' ')
	sb.WriteString(t.Op.Value)
	sb.WriteRune(' ')
	sb.WriteString(rwork)

	// If the lexer did its job, it should only be these discrete values.
	switch t.Op.Value {
	case "+":
		sum = left + right
	case "-":
		sum = left - right
	case "*":
		sum = left * right
	case "/":
		if right == 0 {
			return 0, "ERROR: DIVIDE BY ZERO"
		} else {
			sum = left / right
		}
	default:
		panic("Unreachable! The Lexer failed to validate an Op Token!")
	}

	return sum, sb.String()
}
