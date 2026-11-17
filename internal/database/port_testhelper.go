package database

import (
	"fmt"
	"strconv"
)

// portStringer is the stable port contract shared by Testcontainers port types.
type portStringer interface {
	Port() string
}

// mappedPortInt converts a Testcontainers mapped port to an integer.
func mappedPortInt(port portStringer) (int, error) {
	portString := port.Port()
	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return 0, fmt.Errorf("failed to convert mapped port %q to an integer: %w", portString, err)
	}

	return portInt, nil
}
