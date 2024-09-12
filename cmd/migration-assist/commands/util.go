package commands

import (
	"bufio"
	"fmt"
	"os"
)

func ConfirmationPrompt(question string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", question)

	rd := bufio.NewReader(os.Stdin)
	r, err := rd.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		return false
	}

	if len(r) > 0 && (r[0] == 'y' || r[0] == 'Y') {
		return true
	}

	return false
}
