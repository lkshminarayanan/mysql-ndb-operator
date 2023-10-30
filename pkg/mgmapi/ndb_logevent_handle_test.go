package mgmapi_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mysql/ndb-operator/pkg/mgmapi"
)

const logEventTestConnectString = ":13000"

func createNdbLogEventHandle(ctx context.Context, t *testing.T,
	eventCategoryFilters ...mgmapi.EventCategory) mgmapi.NdbLogEventHandle {
	// create a log event handle
	logEventHandle, err := mgmapi.CreateNdbLogEventHandle(ctx, logEventTestConnectString, eventCategoryFilters...)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			// Management server not running. Skip the test
			t.Skipf("Skipping due to error : %v", err)
		}

		// Connection failing due to some other error
		t.Fatalf("Connection failed: %s", err)
		return nil
	}

	return logEventHandle
}

func TestNdbLogEventHandle_NextEvent(t *testing.T) {
	// Create a logEventHandle
	le := createNdbLogEventHandle(context.Background(), t, mgmapi.EventCategoryCheckpoint)
	defer le.Destroy()

	receivedEventCount := make(map[mgmapi.EventType]int)
	for i := 0; i < 3; i++ {
		if event, err := le.NextEvent(); err == nil {
			receivedEventCount[event.Type]++
			t.Log(event)
		} else {
			t.Error("NextEvent failed : ", err)
		}
	}

	// verify that we got atleast two different types of events
	// i.e. atleast GCPStarted and GCPCompleted
	if len(receivedEventCount) < 2 {
		t.Error("Only one type of event received")
	}
}

func TestNdbLogEventHandle_NextEventWithFilters(t *testing.T) {
	// Create a logEventHandle with subscription to multiple categories
	le := createNdbLogEventHandle(context.Background(), t, mgmapi.EventCategoryCheckpoint, mgmapi.EventCategoryStatistics)
	defer le.Destroy()

	// Test NextEvent with one filter
	for i := 0; i < 2; i++ {
		if event, err := le.NextEvent(mgmapi.EventTypeGlobalCheckpointCompleted); err == nil {
			t.Log(event)
			// error if a different type of event is received
			if event.Type != mgmapi.EventTypeGlobalCheckpointCompleted {
				t.Error("Received an unexpected event type :", event.Type)
			}
		} else {
			t.Error("NextEvent failed : ", err)
		}
	}

	// Test NextEvent with multiple filters
	for i := 0; i < 4; i++ {
		if event, err := le.NextEvent(mgmapi.EventTypeGlobalCheckpointStarted, mgmapi.EventTypeGlobalCheckpointCompleted); err == nil {
			t.Log(event)
			// error if a different type of event is received
			if event.Type != mgmapi.EventTypeGlobalCheckpointStarted && event.Type != mgmapi.EventTypeGlobalCheckpointCompleted {
				t.Error("Received an unexpected event type :", event.Type)
			}
		} else {
			t.Error("NextEvent failed : ", err)
		}
	}
}

func TestNdbLogEventHandle_RegisterEventCallback(t *testing.T) {
	// Create a logEventHandle
	le := createNdbLogEventHandle(context.Background(), t, mgmapi.EventCategoryCheckpoint)
	defer le.Destroy()

	errChan := le.RegisterEventCallback(func(e *mgmapi.Event) error {
		t.Log(e)
		if e.Type != mgmapi.EventTypeGlobalCheckpointCompleted {
			err := fmt.Errorf("RegisterEventCallback executed the callback after receiving a wrong event")
			t.Error(err)
			return err
		}
		return nil
	}, mgmapi.EventTypeGlobalCheckpointCompleted)

	if err := <-errChan; err != nil {
		t.Error("RegisterEventCallback callback failed : ", err)
	}
}

func TestNdbLogEventHandle_RegisterEventCallbackWrapper(t *testing.T) {

	err, errChan := mgmapi.RegisterEventCallback(context.Background(), logEventTestConnectString, func(e *mgmapi.Event) error {
		t.Log(e)
		if e.Type != mgmapi.EventTypeGlobalCheckpointStarted {
			err := fmt.Errorf("RegisterEventCallback executed the callback after receiving a wrong event")
			t.Error(err)
			return err
		}
		return nil
	}, mgmapi.EventTypeGlobalCheckpointCompleted, mgmapi.EventTypeGlobalCheckpointStarted)

	if err != nil {
		t.Error("RegisterEventCallback failed : ", err)
	}

	// check for errors in the callback
	if err := <-errChan; err != nil {
		t.Error("RegisterEventCallback callback failed : ", err)
	}
}

func TestNdbLogEventHandle_WithBackupCommand(t *testing.T) {

	client, err := mgmapi.NewMgmClient(logEventTestConnectString)
	if err != nil {
		t.Fatal("Failed to create management server : ", err)
	}

	// Register callback for when the backup completes
	backupId := new(uint64)
	err, errChan := mgmapi.RegisterEventCallback(context.Background(), logEventTestConnectString, func(e *mgmapi.Event) error {
		t.Log(e)
		if e.Type != mgmapi.EventTypeBackupCompleted || e.Data["backup_id"] != *backupId {
			t.Error("Received unexpected backupId or event type")
		}
		return nil
	}, mgmapi.EventTypeBackupCompleted)

	if err != nil {
		t.Error("RegisterEventCallback failed : ", err)
	}

	// Start the actual backup
	*backupId, err = client.StartBackup()
	if err != nil {
		t.Fatal("Failed to start backup : ", err)
	}

	// check for errors in the callback
	if err := <-errChan; err != nil {
		t.Error("RegisterEventCallback callback failed : ", err)
	}
}
