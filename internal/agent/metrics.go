package agent

import "time"

// BootMetrics records timing and transfer statistics for the strata-agent
// boot sequence. All duration fields are in milliseconds.
//
// The caller of Run() is responsible for populating FetchBytes, CachedLayers,
// and DownloadedLayers from the LayerFetcher's stats after Run() returns.
type BootMetrics struct {
	StartedAt        time.Time `json:"started_at"`
	LockfileMs       int64     `json:"lockfile_ms"`       // step 1: acquire lockfile
	FetchMs          int64     `json:"fetch_ms"`          // step 3: parallel download
	FetchBytes       int64     `json:"fetch_bytes"`       // bytes downloaded (cache misses only)
	MountMs          int64     `json:"mount_ms"`          // step 4: OverlayFS assembly
	ConfigureMs      int64     `json:"configure_ms"`      // step 5: write env config files
	TotalMs          int64     `json:"total_ms"`          // wall time: start → ready
	LayerCount       int       `json:"layer_count"`       // layers in lockfile
	CachedLayers     int       `json:"cached_layers"`     // sqfs already in /strata/cache
	DownloadedLayers int       `json:"downloaded_layers"` // fetched from S3
}
