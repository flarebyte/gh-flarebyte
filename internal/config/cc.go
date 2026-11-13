package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/scanner"
)

// parseCC accepts only one optional positive integer, without CUE expressions.
func parseCC(raw string) (CCConfig, error) {
	cfg := CCConfig{MinLines: 10}
	for _, name := range extractTopLevelFieldNames(raw) {
		if name != "cc" {
			continue
		}
		header := regexp.MustCompile(`(?m)^[ \t]*cc[ \t]*:`).FindStringIndex(raw)
		if header == nil {
			return cfg, fmt.Errorf("cc requires an object")
		}
		value := strings.TrimSpace(raw[header[1]:])
		if !strings.HasPrefix(value, "{") {
			return cfg, fmt.Errorf("cc requires an object")
		}
		end, ok := findMatchingBrace(value, 0)
		if !ok {
			return cfg, fmt.Errorf("unclosed cc object")
		}
		block := value[1:end]
		var s scanner.Scanner
		s.Init(strings.NewReader(block))
		s.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanComments | scanner.SkipComments
		var scanErr error
		s.Error = func(_ *scanner.Scanner, msg string) { scanErr = fmt.Errorf("invalid cc: %s", msg) }
		seen := false
		for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
			if tok == ',' {
				continue
			}
			if tok != scanner.Ident || s.TokenText() != "minLines" || seen {
				return cfg, fmt.Errorf("cc accepts only one minLines field")
			}
			seen = true
			if s.Scan() != ':' || s.Scan() != scanner.Int {
				return cfg, fmt.Errorf("cc.minLines must be a positive integer")
			}
			n, err := strconv.Atoi(s.TokenText())
			if err != nil || n < 1 {
				return cfg, fmt.Errorf("cc.minLines must be a positive integer")
			}
			cfg.MinLines = n
		}
		return cfg, scanErr
	}
	return cfg, nil
}
