package mgmapi

import (
	"fmt"

	"github.com/mysql/ndb-operator/config/debug"
)

// EventCategory constants are the Golang equivalent of ndb_mgm_event_category enums
// defined in storage/ndb/include/mgmapi/ndb_logevent.h in MySQL Cluster source code.
// Note : only a subset of category required by the operator are included here.
type EventCategory uint32

const (
	EventCategoryStatistics EventCategory = 252 // NDB_MGM_EVENT_CATEGORY_STATISTIC
	EventCategoryCheckpoint EventCategory = 253 // NDB_MGM_EVENT_CATEGORY_CHECKPOINT
	EventCategoryBackup     EventCategory = 261 // NDB_MGM_EVENT_CATEGORY_BACKUP
)

func (et EventCategory) String() string {
	switch et {
	case EventCategoryStatistics:
		return "Statistics"
	case EventCategoryCheckpoint:
		return "Checkpoint"
	case EventCategoryBackup:
		return "Backup"
	default:
		debug.Panic("unsupported EventCategory")
		return fmt.Sprintf("Unsupported EventCategory(=%d)", et)
	}
}

// EventType constants are the Golang equivalent of Ndb_logevent_type enums
// defined in storage/ndb/include/mgmapi/ndb_logevent.h in MySQL Cluster source code.
// Note : only a subset of events required by the operator are included here.
type EventType uint32

// Note : when you add new type here, update the map in GetEventCategory accordingly
const (
	// EventCategoryCheckpoint
	EventTypeGlobalCheckpointStarted   EventType = 4 // NDB_LE_GlobalCheckpointStarted
	EventTypeGlobalCheckpointCompleted EventType = 5 // NDB_LE_GlobalCheckpointCompleted
	// EventCategoryBackup
	EventTypeBackupStarted       EventType = 54 // NDB_LE_BackupStarted
	EventTypeBackupFailedToStart EventType = 55 // NDB_LE_BackupFailedToStart
	EventTypeBackupCompleted     EventType = 56 // NDB_LE_BackupCompleted
	EventTypeBackupAborted       EventType = 57 // NDB_LE_BackupAborted
	EventTypeBackupStatus        EventType = 62 // NDB_LE_BackupStatus
	EventTypeRestoreMetaData     EventType = 63 // NDB_LE_RestoreMetaData
	EventTypeRestoreData         EventType = 64 // NDB_LE_RestoreData
	EventTypeRestoreLog          EventType = 65 // NDB_LE_RestoreLog
	EventTypeRestoreStarted      EventType = 66 // NDB_LE_RestoreStarted
	EventTypeRestoreCompleted    EventType = 67 // NDB_LE_RestoreCompleted
)

func (et EventType) String() string {
	switch et {
	case EventTypeGlobalCheckpointStarted:
		return "GlobalCheckPointStarted"
	case EventTypeGlobalCheckpointCompleted:
		return "GlobalCheckPointCompleted"
	case EventTypeBackupStarted:
		return "BackupStarted"
	case EventTypeBackupFailedToStart:
		return "BackupFailedToStart"
	case EventTypeBackupCompleted:
		return "BackupCompleted"
	case EventTypeBackupAborted:
		return "BackupAborted"
	case EventTypeBackupStatus:
		return "BackupStatus"
	case EventTypeRestoreMetaData:
		return "RestoreMetaData"
	case EventTypeRestoreData:
		return "RestoreData"
	case EventTypeRestoreLog:
		return "RestoreLog"
	case EventTypeRestoreStarted:
		return "RestoreStarted"
	case EventTypeRestoreCompleted:
		return "RestoreCompleted"
	default:
		debug.Panic("unsupported EventType")
		return fmt.Sprintf("Unsupported EventType(=%d)", et)
	}
}

// getEventCategory returns the EventCategory for a given EventType
func (et EventType) GetEventCategory() EventCategory {
	return map[EventType]EventCategory{
		// Add event => category mappings here when adding new events/categories to this file
		// EventCategoryCheckpoint
		EventTypeGlobalCheckpointStarted:   EventCategoryCheckpoint,
		EventTypeGlobalCheckpointCompleted: EventCategoryCheckpoint,
		// EventCategoryBackup
		EventTypeBackupStarted:       EventCategoryBackup,
		EventTypeBackupFailedToStart: EventCategoryBackup,
		EventTypeBackupCompleted:     EventCategoryBackup,
		EventTypeBackupAborted:       EventCategoryBackup,
		EventTypeBackupStatus:        EventCategoryBackup,
		EventTypeRestoreMetaData:     EventCategoryBackup,
		EventTypeRestoreData:         EventCategoryBackup,
		EventTypeRestoreLog:          EventCategoryBackup,
		EventTypeRestoreStarted:      EventCategoryBackup,
		EventTypeRestoreCompleted:    EventCategoryBackup,
	}[et]
}

// every event will start with eventHeader and end with new line
const (
	eventHeader = "log event reply"
	eventFooter = ""
)
