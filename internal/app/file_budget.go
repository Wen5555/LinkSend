package app

import "github.com/Wen5555/LinkSend/internal/transfer"

// All services/identities in this process share this budget. Creating a new
// task, peer or profile does not replenish it. Desktop persists the preference.
var processFileBudget = transfer.NewFileBodyLimiter()

func ConfigureFileSendLimit(bytesPerSecond int64) error {
	return processFileBudget.SetRate(bytesPerSecond)
}

func FileSendLimit() int64 { return processFileBudget.Rate() }
