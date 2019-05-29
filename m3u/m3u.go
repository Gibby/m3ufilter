package m3u

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/hoshsadiq/m3ufilter/config"
	"io"
	"strings"
)

type Stream struct {
	Duration string
	Name     string
	Uri      string

	// these are attributes
	Id    string
	Shift string
	Logo  string
	Group string
}

func decode(reader io.Reader, providerConfig *config.Provider) ([]*Stream, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return nil, err
	}

	var eof bool
	var streams []*Stream

	lines := 0
	for !eof {
		var extinfLine string
		var urlLine string

		for !eof && !strings.HasPrefix(extinfLine, "#EXTINF:") {
			extinfLine, eof = getLine(buf)
		}
		if eof {
			break
		}

		urlLine, eof = getLine(buf)
		if eof {
			break
		}

		lines++
		if (lines % 1000) == 0 {
			log.Infof("Parsing %d streams", lines)
		}

		stream, err := parseExtinfLine(extinfLine, urlLine)
		if err != nil {
			return nil, err
		}

		if !shouldIncludeSegment(stream, providerConfig.Filters) {
			continue
		}

		setSegmentValues(stream, providerConfig.Setters)

		streams = append(streams, stream)
	}

	log.Infof("Found %d valid streams", len(streams))

	return streams, nil
}

func getLine(buf *bytes.Buffer) (string, bool) {
	var eof bool
	var line string
	var err error
	for !eof {
		line, err = buf.ReadString('\n')
		if err == io.EOF {
			eof = true
		} else if err != nil {
			panic("something went wrong")
		}

		if len(line) < 1 || line == "\r" {
			continue
		}
		break
	}
	return line, eof
}

func parseExtinfLine(attrline string, urlLine string) (*Stream, error) {
	attrline = strings.TrimSpace(attrline)
	urlLine = strings.TrimSpace(urlLine)

	stream := &Stream{Uri: urlLine}
	state := "duration"
	key := ""
	current := ""
	quote := "\""
	escapeNext := false
	for i := 8; i < len(attrline); i++ {
		c := attrline[i]

		if state == "quotes" {
			if string(c) != quote {
				current += string(c)
			} else {
				switch key {
				case "tvg-id":
					stream.Id = current
				case "tvg-shift":
					stream.Shift = current
				case "tvg-name":
					stream.Name = current
				case "tvg-logo":
					stream.Logo = current
				case "group-title":
					stream.Group = current
				}

				key = ""
				current = ""
				state = "start"
			}
			continue
		} else if state == "name" {
			stream.Name += string(c)
			continue
		}

		if escapeNext {
			if state == "duration" {
				stream.Duration += string(c)
			} else if state == "keyname" {
				key += string(c)
			}

			escapeNext = false
			continue
		}

		if c == '\\' {
			escapeNext = true
			continue
		}

		if c == '"' || c == '\'' {
			if state != "value" {
				return nil, errors.New(fmt.Sprintf("Unexpected character '%s' found, expected '=' for key %s on position %d in line: %s", string(c), key, i, attrline))
			}
			state = "quotes"
			quote = string(c)
			continue
		}

		if c == ',' {
			state = "name"
			break
		}

		if state == "keyname" {
			if c == ' ' || c == '\t' {
				key = ""
				state = "start"
			} else if c == '=' {
				state = "value"
			} else {
				key += string(c)
			}
			continue
		}

		if state == "duration" {
			if (c >= 48 && c <= 57) || c == '.' || c == '-' {
				stream.Duration += string(c)
				continue
			}
		}

		if c != ' ' && c != '\t' {
			state = "keyname"
			key += string(c)
		}
	}

	if state == "keyname" && current == "" {
		return nil, errors.New(fmt.Sprintf("Key %s started but no value assigned on line: %s", key, attrline))
	}

	if state == "quotes" {
		return nil, errors.New(fmt.Sprintf("Unclosed quote on line: %s", attrline))
	}

	return stream, nil
}