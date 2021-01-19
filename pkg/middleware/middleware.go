package middleware

import (
	"time"

	"github.com/go-logr/logr"
)

func Period(
	period time.Duration,
	log logr.Logger,
	next func(logr.Logger),
) {
	for {
		next(log)

		time.Sleep(period)
	}
}
