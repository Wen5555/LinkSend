package app

import (
	"strings"
)

func terminalQueue(state string) bool {
	return state == "completed" || state == "cancelled" || state == "expired"
}

// ClearTransferQueue clears presentation only. Durable history and received
// files remain untouched. With all=true it first cancels this snapshot's work;
// concurrent new arrivals are not part of the command and remain visible.
func (s *Service) ClearTransferQueue(all bool) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	items, err := s.queueItems()
	if err != nil {
		return err
	}
	tasks := s.Tasks()
	completedTasks := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		completedTasks[task.ID] = task.State == "completed" || task.State == "no_content"
	}
	if all {
		for _, item := range items {
			if !terminalQueue(item.State) {
				if e := s.CancelQueue(item.ID, item.Revision); e != nil {
					return e
				}
			}
		}
		for _, task := range tasks {
			current, ok := s.Task(task.ID)
			if ok && !isTerminal(current.State) && current.State != "cancel_requested" {
				if e := s.CancelTask(task.ID); e != nil {
					return e
				}
			}
		}
	}
	tx, err := s.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range items {
		if all || item.State == "completed" || completedTasks[item.TaskID] {
			if _, err = tx.Exec(`INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "transfer_view/queue/"+item.ID, "1"); err != nil {
				return err
			}
		}
	}
	for _, task := range tasks {
		if all || task.State == "completed" || task.State == "no_content" {
			if _, err = tx.Exec(`INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "transfer_view/task/"+task.ID, task.AttemptID); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.notifyChange()
	return nil
}

func (s *Service) filterTransferView(queue []QueueItem, tasks []TaskSnapshot) ([]QueueItem, []TaskSnapshot, error) {
	rows, err := s.store.db.Query(`SELECT key,value FROM metadata WHERE key LIKE 'transfer_view/%'`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	hidden := map[string]string{}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return nil, nil, err
		}
		hidden[strings.TrimPrefix(key, "transfer_view/")] = value
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	visibleQueue := make([]QueueItem, 0, len(queue))
	for _, item := range queue {
		if _, exists := hidden["queue/"+item.ID]; !exists || !terminalQueue(item.State) {
			visibleQueue = append(visibleQueue, item)
		}
	}
	visibleTasks := make([]TaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		attempt, exists := hidden["task/"+task.ID]
		if !exists || attempt != task.AttemptID || !isTerminal(task.State) {
			visibleTasks = append(visibleTasks, task)
		}
	}
	return visibleQueue, visibleTasks, nil
}
