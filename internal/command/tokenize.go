package command

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/joeycumines/one-shot-man/internal/tokenizer"
)

// TokenizeCommand implements the "osm tokenize" CLI command.
type TokenizeCommand struct {
	BaseCommand
	stdin   *bool
	quiet   *bool
	verbose *bool
}

// NewTokenizeCommand creates a new TokenizeCommand.
func NewTokenizeCommand() *TokenizeCommand {
	return &TokenizeCommand{
		BaseCommand: *NewBaseCommand(
			"tokenize",
			"Tokenize text using a HuggingFace tokenizer JSON file. Outputs token IDs and optionally token values and byte offsets.",
			"osm tokenize -tokenizer <path> [-text <text> | -stdin] [-quiet] [-verbose]",
		),
	}
}

// SetupFlags registers the command-specific flags.
func (c *TokenizeCommand) SetupFlags(fs *flag.FlagSet) {
	fs.String("tokenizer", "", "Path to HuggingFace tokenizer.json file (required)")
	fs.String("text", "", "Input text to tokenize")
	c.stdin = fs.Bool("stdin", false, "Read input text from standard input")
	c.quiet = fs.Bool("quiet", false, "Output only the token count")
	c.verbose = fs.Bool("verbose", false, "Output full token info: ID, value, byte offsets")
}

// Execute runs the tokenization command.
func (c *TokenizeCommand) Execute(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tokenize", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	c.SetupFlags(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}

	tokenizerPath := fs.Lookup("tokenizer").Value.String()
	if tokenizerPath == "" {
		return fmt.Errorf("tokenizer flag is required: -tokenizer <path>")
	}

	var text string
	fromStdin := *c.stdin
	textExplicitlySet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "text" {
			textExplicitlySet = true
		}
	})
	textFlag := fs.Lookup("text").Value.String()

	if fromStdin && textExplicitlySet {
		return fmt.Errorf("cannot specify both -text and -stdin")
	}

	if textExplicitlySet {
		text = textFlag
	} else if fromStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		text = string(data)
	} else {
		// If neither specified, read from stdin by default
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		text = string(data)
	}

	tok, err := tokenizer.LoadTokenizerFromFile(tokenizerPath)
	if err != nil {
		return fmt.Errorf("loading tokenizer: %w", err)
	}

	tokens, count, err := tok.Encode(text)
	if err != nil {
		return fmt.Errorf("tokenizing: %w", err)
	}

	quiet := *c.quiet
	verbose := *c.verbose

	if quiet {
		fmt.Fprintf(stdout, "%d\n", count)
		return nil
	}

	for _, token := range tokens {
		if verbose {
			fmt.Fprintf(stdout, "%d\t%s\t[%d, %d]\n",
				token.ID, token.Value, token.Offsets[0], token.Offsets[1])
		} else {
			fmt.Fprintf(stdout, "%d\n", token.ID)
		}
	}

	return nil
}
