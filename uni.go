// Command uni prints Unicode information about characters.
package main // import "arp242.net/uni"

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"arp242.net/uni/unidata"
)

var (
	errFlag      = errors.New("")
	errNoMatches = errors.New("no matches")
)

var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	exit             = os.Exit
)

const usagetext = `Usage: %s [-hrq] [help | identify | search | print | emoji]

Flags:
    -q      Quiet output; don't print header, "no matches", etc.
    -r      "Raw" output instead of displaying graphical variants for control
            characters and ◌ (U+25CC) before combining characters.

Commands:
    identify [string string ...]
        Idenfity all the characters in the given strings.

    search [word word ...]
        Search description for any of the words.

    print [ident ident ...]
        Print characters by codepoint, category, or block:

            Codepoints             U+2042, U+2042..U+2050
            Categories and Blocks  OtherPunctuation, Po, GeneralPunctuation
            all                    Everything

        Names are matched case insensitive; spaces and commas are optional and
        can be replaced with an underscore. "Po", "po", "punction, OTHER",
        "Punctuation_other", and PunctuationOther are all identical.

    emoji [-tone tone] [-gender gender,...] [word word ...]
        Print emojis by group name:

             all              Everything.
             groups           All group and subgroup names.
             <anything else>  Emojis matching the group or subgroup.

        -tone can be light, mediumlight, medium, mediumdark, dark.
        -gender is a comma-separated list of person, man, or woman.

        Note: output may contain unprintable character (U+200D and U+FE0F) which
        may not survive a select and copy operation from text-based applications
        such as terminals. It's recommended to copy to the clipboard directly
        with e.g. xclip.
`

func usage(err error) {
	out := stdout
	e := 0
	if err != nil {
		out = stderr
		e = 1
		if err != errFlag {
			_, _ = fmt.Fprintf(out, "%s: error: %v\n", os.Args[0], err)
		}
	}

	_, _ = fmt.Fprintf(out, usagetext, os.Args[0])
	exit(e)
}

func main() {
	var (
		//output string
		quiet bool
		help  bool
		raw   bool
	)
	// TODO: Output format; valid values are human (default), csv, tsv, json.
	// TODO: Add option to configure columns.
	//flag.StringVar(&output, "o", "human", "")
	flag.BoolVar(&quiet, "q", false, "")
	flag.BoolVar(&help, "h", false, "")
	flag.BoolVar(&raw, "r", false, "")
	flag.Usage = func() { usage(errFlag) }
	flag.Parse()

	if help {
		usage(nil)
	}

	args := flag.Args()
	if len(args) == 0 {
		usage(errors.New("no command given"))
	}

	var err error
	switch strings.ToLower(args[0]) {
	default:
		usage(fmt.Errorf("unknown command: %q", args[0]))
	case "help", "h":
		usage(nil)
	case "identify", "i":
		err = identify(getargs(args[1:], quiet), quiet, raw)
	case "search", "s":
		err = search(getargs(args[1:], quiet), quiet, raw)
	case "print", "p":
		err = print(getargs(args[1:], quiet), quiet, raw)
	case "emoji", "e":
		err = emoji(getargs(args[1:], quiet), quiet, raw)
	}
	if err == errNoMatches && quiet {
		err = nil
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		exit(1)
	}
}

// Use commandline args or stdin.
func getargs(args []string, quiet bool) []string {
	if len(args) > 0 {
		return args
	}

	// Print message so people aren't left waiting when typing "uni print". We
	// don't print a newline and a \r later on, so you don't see it in actual
	// pipe usage, just when it would "hang" uni.
	if !quiet && isTerm {
		fmt.Fprintf(stderr, "uni: reading from stdin...")
	}
	stdin, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		panic(fmt.Errorf("read stdin: %s", err))
	}
	if !quiet && isTerm {
		fmt.Fprintf(stderr, "\r")
	}

	return strings.Split(strings.TrimRight(string(stdin), "\n"), "\n")
}

func search(args []string, quiet, raw bool) error {
	var na []string
	for _, a := range args {
		if a != "" {
			na = append(na, a)
		}
	}
	args = na
	if len(args) == 0 {
		return errors.New("search: need search term")
	}

	var out printer
	words := make([]string, len(args))
	for i := range args {
		words[i] = strings.ToUpper(args[i])
	}
	for _, info := range unidata.Codepoints {
		m := 0
		for _, w := range words {
			if strings.Contains(info.Name, w) {
				m++
			}
		}
		if m == len(words) {
			out = append(out, info)
		}
	}

	if len(out) == 0 {
		return errNoMatches
	}

	out.PrintSorted(stdout, quiet, raw)
	return nil
}

func emoji(args []string, quiet, raw bool) error {
	subflag := flag.NewFlagSet("emoji", flag.ExitOnError)
	tone := subflag.String("tone", "", "Skin tone; light, mediumlight, medium, mediumdark, or dark")
	gender := subflag.String("gender", "", "comma-separated list of genders to include (man, woman, person); default is all")
	subflag.Parse(args)

	switch *tone {
	case "":
	case "light":
		*tone = "\U0001f3fb"
	case "mediumlight":
		*tone = "\U0001f3fc"
	case "medium":
		*tone = "\U0001f3fd"
	case "mediumdark":
		*tone = "\U0001f3fe"
	case "dark":
		*tone = "\U0001f3ff"
	default:
		fmt.Fprintf(stderr, "uni: invalid skin tone: %q\n", *tone)
		flag.Usage()
		exit(55)
	}

	genders := []string{"person", "man", "woman"}
	if *gender != "" {
		genders = strings.Split(*gender, ",")
		for i, g := range genders {
			switch g {
			case "p", "people":
				g = "person"
			case "men", "m", "male":
				g = "man"
			case "women", "w", "female":
				g = "woman"
			}
			genders[i] = g
		}
	}

	out := [][]string{}
	cols := []int{4, 0, 0, 0}
	add := func(e unidata.Emoji, c string) {
		out = append(out, []string{c, e.Name, e.Group, e.Subgroup})
		if l := utf8.RuneCountInString(e.Name); l > cols[1] {
			cols[1] = l
		}
		if l := utf8.RuneCountInString(e.Group); l > cols[2] {
			cols[2] = l
		}
		if l := utf8.RuneCountInString(e.Subgroup); l > cols[3] {
			cols[3] = l
		}
	}

	for _, a := range subflag.Args() {
		a = strings.ToLower(a)
		switch a {
		case "all":
			a = ""
		case "groups":
			for _, g := range unidata.EmojiGroups {
				fmt.Fprintln(stdout, g)
				for _, sg := range unidata.EmojiSubgroups[g] {
					fmt.Fprintln(stdout, "   ", sg)
				}
			}
			return nil
		}

		found := false
		for _, e := range unidata.Emojis {
			if !strings.Contains(strings.ToLower(e.Group), a) &&
				!strings.Contains(strings.ToLower(e.Subgroup), a) {
				continue
			}

			found = true

			c := e.String()

			// 1F9D8                            # 🧘 E5.0 person in lotus position
			// 1F9D8 1F3FF 200D 2642 FE0F       # 🧘🏿‍♂️ E5.0 man in lotus position: dark skin tone
			//
			// 1F9D1 200D 2695 FE0F             # 🧑‍⚕️ E12.1 health worker
			// 1F9D1 1F3FF 200D 2695 FE0F       # 🧑🏿‍⚕️ E12.1 health worker: dark skin tone
			// 1F468 200D 2695 FE0F             # 👨‍⚕️ E4.0 man health worker
			// 1F468 1F3FF 200D 2695 FE0F       # 👨🏿‍⚕️ E4.0 man health worker: dark skin tone
			//
			// 1F470                            # 👰 E2.0 bride with veil
			// 1F470 1F3FF                      # 👰🏻 E2.0 bride with veil: dark skin tone
			//
			// 1F575 FE0F                       # 🕵️ E2.0 detective
			// 1F575 1F3FF                      # 🕵🏿 E2.0 detective: dark skin tone
			// 1F575 FE0F 200D 2642 FE0F        # 🕵️‍♂️ E4.0 man detective
			// 1F575 1F3FF 200D 2642 FE0F       # 🕵🏿‍♂️ E4.0 man detective: dark skin tone
			if *tone != "" && e.SkinTones {
				var ns string
				i := 0
				for _, r := range c {
					switch i {
					case 0:
						ns = string(r)
					case 1:
						ns += "\u200d" + *tone
						fallthrough
					default:
						ns += string(r)
					}
					i++
				}
				c = ns
				if i == 1 {
					c += "\u200d" + *tone
				}
			}

			// No genders: append and stop here.
			if e.Genders == unidata.GenderNone {
				add(e, c)
				continue
			}

			for _, g := range genders {
				if e.Genders == unidata.GenderSign {
					switch g {
					case "person":
						add(e, c)
					case "woman":
						ee := e
						ee.Name = strings.Replace(ee.Name, "person", "woman", 1)
						add(ee, c+"\u200d\u2640\ufe0f")
					case "man":
						ee := e
						ee.Name = strings.Replace(ee.Name, "person", "man", 1)
						add(ee, c+"\u200d\u2642\ufe0f")
					}
				} else if e.Genders == unidata.GenderRole {
					switch g {
					case "person":
						add(e, c)
					case "woman":
						ee := e
						ee.Name = "woman " + ee.Name
						add(ee, "\U0001f469"+c[4:])
					case "man":
						ee := e
						ee.Name = "man " + ee.Name
						add(ee, "\U0001f468"+c[4:])
					}
				}
			}
		}

		if !found {
			return fmt.Errorf("no such emoji group or subgroup: %q", a)
		}
	}

	// TODO: not always correctly aligned as some emojis are double-width and
	// some are not. As far as I can tell, there is no good way to predict this
	// as it will depend on the font. Unicode recommends "emoji presentation
	// sequences behave as though they were East Asian Wide", but that's too
	// simplistic too.
	for _, o := range out {
		for i, c := range o {
			if i == 0 {
				fmt.Fprintf(stdout, c+" ")
			} else {
				fmt.Fprint(stdout, fill(c, cols[i]+2))
			}
		}
		fmt.Fprintln(stdout, "")
	}
	return nil
}

func print(args []string, quiet, raw bool) error {
	var out printer

	for _, a := range args {
		canon := unidata.CanonicalCategory(a)

		// Print everything.
		if canon == "all" {
			for _, info := range unidata.Codepoints {
				out = append(out, info)
			}
			continue
		}

		// Category name.
		if cat, ok := unidata.Catmap[canon]; ok {
			for _, info := range unidata.Codepoints {
				if info.Cat == cat {
					out = append(out, info)
				}
			}
			continue
		}

		// Block.
		if bl, ok := unidata.Blockmap[canon]; ok {
			for cp := unidata.Blocks[bl][0]; cp <= unidata.Blocks[bl][1]; cp++ {
				s, ok := unidata.Codepoints[fmt.Sprintf("%04X", cp)]
				if ok {
					out = append(out, s)
				}
			}
			continue
		}

		// U2042, U+2042, U+2042..U+2050, 2042..2050
		if strings.HasPrefix(canon, "u") || strings.Contains(canon, "..") {
			canon = strings.ToUpper(canon)

			s := strings.Split(canon, "..")
			switch len(s) {
			case 1:
				s = append(s, s[0])
			case 2:
				// Do nothing
			default:
				return fmt.Errorf("unknown ident: %q", a)
			}

			start, err := strconv.ParseInt(strings.TrimLeft(strings.TrimLeft(s[0], "U"), "+"), 16, 64)
			if err != nil {
				return err
			}
			end, err := strconv.ParseInt(strings.TrimLeft(strings.TrimLeft(s[1], "U"), "+"), 16, 64)
			if err != nil {
				return err
			}

			for i := start; i <= end; i++ {
				info, ok := unidata.FindCodepoint(rune(i))
				if !ok {
					return fmt.Errorf("unknown codepoint: U+%.4X", i)
				}
				out = append(out, info)
			}

			continue
		}

		return fmt.Errorf("unknown identifier: %q", a)
	}

	out.PrintSorted(stdout, quiet, raw)
	return nil
}

func identify(ins []string, quiet, raw bool) error {
	in := strings.Join(ins, "")
	if !utf8.ValidString(in) {
		_, _ = fmt.Fprintf(stderr, "uni: WARNING: input string is not valid UTF-8\n")
	}

	var out printer
	for _, c := range in {
		info, ok := unidata.FindCodepoint(c)
		if !ok {
			return fmt.Errorf("unknown codepoint: %.4X", c)
		}

		out = append(out, info)
	}

	out.Print(stdout, quiet, raw)
	return nil
}

func fmtChar(c rune, raw bool) string {
	if raw {
		return string(c)
	}

	// Display combining characters with ◌.
	if unicode.In(c, unicode.Mn, unicode.Mc, unicode.Me) {
		return "\u25cc" + string(c)
	}

	switch {
	case unicode.IsControl(c):
		switch {
		case c < 0x20: // C0; use "Control Pictures" block
			c += 0x2400
		case c == 0x7f: // DEL
			c = 0x2421
		// No control pictures for C1 or anything else, use "open box".
		default:
			c = 0x2423
		}
	// "Other, Format" category except the soft hyphen and spaces.
	case !unicode.IsPrint(c) && c != 0x00ad && !unicode.In(c, unicode.Zs):
		c = 0xfffd
	}

	return string(c)
}
