package model

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type ID string

func NewID() ID {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return ID(hex.EncodeToString(b[:]))
}

type Operation string
type Ship struct {
	Identity             string    `json:"identity,omitempty"`
	PierBytes            *int64    `json:"pierBytes,omitempty"`
	MeasuredAt           time.Time `json:"measuredAt,omitempty"`
	RuntimeVersion       string    `json:"runtimeVersion,omitempty"`
	ID                   ID        `json:"id"`
	Name                 string    `json:"name"`
	Nickname             string    `json:"nickname"`
	PierPath             string    `json:"pierPath"`
	PierDeletionIdentity string    `json:"pierDeletionIdentity,omitempty"`
	RuntimePath          string    `json:"runtimePath"`
	LoomExponent         int       `json:"loomExponent"`
	HTTPBindAddress      string    `json:"httpBindAddress"`
	WebPort              int       `json:"webPort"`
	DesiredRunning       bool      `json:"desiredRunning"`
	RecoveryBlocked      bool      `json:"recoveryBlocked"`
	RecoveryBlockReason  string    `json:"recoveryBlockReason"`
	State                string    `json:"state"`
	Error                string    `json:"error"`
}
type Job struct {
	Retryable      bool      `json:"retryable,omitempty"`
	Result         string    `json:"result,omitempty"`
	ID             ID        `json:"id"`
	ShipID         ID        `json:"shipId"`
	Kind           Operation `json:"kind"`
	Trigger        string    `json:"trigger"`
	ScheduleID     ID        `json:"scheduleId,omitempty"`
	OccurrenceID   ID        `json:"occurrenceId,omitempty"`
	DependsOnJobID ID        `json:"dependsOnJobId,omitempty"`
	Phase          string    `json:"phase"`
	PriorRunning   bool      `json:"priorRunning"`
	UploadID       ID        `json:"uploadId,omitempty"`
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	ExitCode       *int      `json:"exitCode,omitempty"`
	Error          string    `json:"error"`
}
type JobRequest struct {
	ShipID   ID
	Kind     Operation
	Trigger  string
	UploadID ID
}
type ProcessRecord struct {
	Operation     Operation `json:"operation"`
	LaunchID      ID        `json:"launchId"`
	JobID         ID        `json:"jobId"`
	ShipID        ID        `json:"shipId"`
	Kind          string    `json:"kind"`
	Phase         string    `json:"phase"`
	PID           int       `json:"pid"`
	StartIdentity string    `json:"startIdentity"`
	BinaryDigest  string    `json:"binaryDigest"`
	ExitCode      *int      `json:"exitCode,omitempty"`
}
type Schedule struct {
	ID              ID        `json:"id"`
	ShipID          ID        `json:"shipId"`
	Kind            Operation `json:"kind"`
	AnchorDate      string    `json:"anchorDate"`
	IntervalDays    int       `json:"intervalDays"`
	LocalTime       string    `json:"localTime"`
	Timezone        string    `json:"timezone"`
	Enabled         bool      `json:"enabled"`
	AfterScheduleID ID        `json:"afterScheduleId,omitempty"`
	NextDue         time.Time `json:"nextDue"`
	LastOccurrence  ID        `json:"lastOccurrence,omitempty"`
	LastJobID       ID        `json:"lastJobId,omitempty"`
	LastResult      string    `json:"lastResult"`
}
type Occurrence struct {
	ID           ID
	ScheduleID   ID
	ScheduledFor time.Time
	LocalDate    string
	JobID        ID
}
type Event struct {
	ID     uint64 `json:"id"`
	Type   string `json:"type"`
	ShipID ID     `json:"shipId,omitempty"`
	JobID  ID     `json:"jobId,omitempty"`
	Data   any    `json:"data,omitempty"`
}

type Backup struct {
	ID          ID        `json:"id"`
	ShipID      ID        `json:"shipId"`
	ArchivePath string    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
	Verified    bool      `json:"verified"`
}
