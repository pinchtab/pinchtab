package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
)

type menuOption struct {
	label string
	value string
}

func promptSelect(title string, options []menuOption) (string, error) {
	if len(options) == 0 {
		return "", nil
	}

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, title))
	for i, option := range options {
		fmt.Printf("  %d. %s\n", i+1, option.label)
	}
	fmt.Print(cli.StyleStdout(cli.MutedStyle, "Select an option") + " " + cli.StyleStdout(cli.MutedStyle, "(blank to exit): "))

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(input) == "" {
		return "", err
	}

	choice := strings.TrimSpace(input)
	if choice == "" {
		return "", nil
	}

	if idx, err := strconv.Atoi(choice); err == nil {
		if idx >= 1 && idx <= len(options) {
			return options[idx-1].value, nil
		}
		return "", fmt.Errorf("invalid selection %q", choice)
	}

	for _, option := range options {
		if choice == option.value {
			return option.value, nil
		}
	}

	return "", fmt.Errorf("invalid selection %q", choice)
}

func promptInput(prompt, defaultValue string) (string, error) {
	if prompt == "" {
		// The caller already rendered the prompt.
	} else if defaultValue != "" {
		fmt.Printf("%s %s ", prompt, cli.StyleStdout(cli.MutedStyle, "["+defaultValue+"]"))
	} else {
		fmt.Print(prompt + " ")
	}

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(input) == "" {
		return "", err
	}

	value := strings.TrimSpace(input)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func promptInputHiddenDefault(prompt, defaultValue string) (string, error) {
	if prompt != "" {
		fmt.Print(prompt + " ")
	}

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(input) == "" {
		return "", err
	}

	value := strings.TrimSpace(input)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}
