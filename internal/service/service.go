// Package service infers the developer-facing service behind a listening
// port — "this is your Vite dev server", not just "node, pid 8123". It sits
// one layer above internal/portscan and internal/process: every Detector
// works from a Signal assembled once from data those packages already
// collected, and never does its own process/port discovery or shells out.
//
// Detection is deliberately evidence-based rather than a hardcoded name
// database: a small, extensible set of Detectors each look for one kind of
// service, and Identify picks the most confident match. A Detector that
// isn't sure never guesses — Identify's zero-confidence fallback ("Unknown
// service") is the intended, honest result for anything genuinely
// unrecognized, per the project's existing "clearly indicate unavailable
// information rather than crashing [or guessing]" convention (see
// cmd/portctl/tree.go).
package service

// Source classifies where a detected service appears to run.
type Source string

const (
	SourceLocal   Source = "LOCAL"   // an ordinary process on this machine
	SourceDocker  Source = "DOCKER"  // reached via a Docker-managed proxy/runtime process
	SourceSystem  Source = "SYSTEM"  // a known OS/session daemon, not a dev service
	SourceUnknown Source = "UNKNOWN" // no evidence either way
)

// Signal is the evidence available to a Detector: everything portctl has
// already collected about the process behind one listening port. Detectors
// only read from it — they never fetch anything themselves.
type Signal struct {
	Proto       string
	Port        uint16
	PID         int
	ProcessName string // resolved process name (may be a full path)
	Cmdline     string // full invocation, when resolvable
	Cwd         string
}

// Detection is a Detector's conclusion about a Signal.
type Detection struct {
	Name       string
	Source     Source
	Confidence int // higher wins when more than one Detector matches; 0 = no opinion
}

// Detector recognizes one family of developer services from a Signal.
// Detect returns ok=false when sig shows no evidence of the kind of
// service this Detector looks for — it must never fabricate a guess just
// to return something.
type Detector interface {
	Detect(Signal) (Detection, bool)
}

// Detectors is the ordered, extensible registry `services` and Identify
// run. It's a plain var (not built via init()) so tests can construct
// their own Identify-equivalent over a fixed subset instead of depending
// on hidden package-level registration order.
var Detectors = []Detector{
	dockerDetector{},
	databaseDetector{},
	nodeDetector{},
	goDetector{},
	javaDetector{},
	pythonDetector{},
	systemDetector{},
}

// unknownService is the fallback Identify returns when nothing recognizes
// a Signal — an explicit "we don't know", never an incorrect specific
// claim.
var unknownService = Detection{Name: "Unknown service", Source: SourceUnknown, Confidence: 0}

// Identify runs every registered Detector against sig and returns the
// highest-confidence match. It always returns something displayable —
// unknownService when nothing matched, never a zero Detection.
func Identify(sig Signal) Detection {
	return IdentifyWith(sig, Detectors)
}

// IdentifyWith is Identify parameterized over an explicit detector list,
// so tests can exercise one Detector (or a controlled combination) without
// the full registry.
func IdentifyWith(sig Signal, detectors []Detector) Detection {
	best := unknownService
	for _, d := range detectors {
		det, ok := d.Detect(sig)
		if !ok {
			continue
		}
		if det.Confidence > best.Confidence {
			best = det
		}
	}
	return best
}
