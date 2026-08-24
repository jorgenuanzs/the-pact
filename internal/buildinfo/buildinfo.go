package buildinfo

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

const (
	ProtocolVersion    = 2
	MinProtocolVersion = 1
)

type Info struct {
	Version            string `json:"version"`
	Commit             string `json:"commit"`
	Date               string `json:"date"`
	ProtocolVersion    int    `json:"protocol_version"`
	MinProtocolVersion int    `json:"min_protocol_version"`
}

func Current() Info {
	return Info{
		Version:            Version,
		Commit:             Commit,
		Date:               Date,
		ProtocolVersion:    ProtocolVersion,
		MinProtocolVersion: MinProtocolVersion,
	}
}
