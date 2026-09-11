//go:build !linux

package supervisor

import "context"

func displaySessionStatus() DisplaySessionStatus {
	return DisplaySessionStatus{Ready: true, Type: "desktop"}
}

func waitForDisplaySession(context.Context) (DisplaySessionStatus, error) {
	return displaySessionStatus(), nil
}
