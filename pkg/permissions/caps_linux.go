package permissions

import (
	"os"

	"github.com/go-logr/logr"
	. "github.com/syndtr/gocapability/capability"
)

/* Lol capabilities
* - "ping-sockets" don't need any, just some OS config, so we try that first.
* - the fallback of a raw socket needs CAP_NET_RAW, which we will have if
*   - we're running as root (including setuid root)
*   - NET_RAW is in our binary's `permitted` set, or the process's `ambient` set, AND the binary's `effective` bit is set
* - if not, we can ask for NET_RAW to be added to our effective caps, which is what this function does. For that to work, it needs to be in the permitted set (qv the conditions for that) - all we're asking to do is use it for a while (which the effective file flag does for the static lifetime)
 */
func ApplyNetRaw(log logr.Logger) {
	log.Info("Trying best-effort attempt to get effective CAP_NET_RAW. This operation, and/or this capability, might not be necessary, there are currently no checks.")

	logFileCaps(log)

	log.WithValues("pid (self)", os.Getpid())
	caps, err := NewPid2(0)
	if err != nil {
		log.Error(err, "Can't construct caps object for current process")
		return
	}
	err = caps.Load()
	log.Info("Caps", "original", caps.String())
	if err != nil {
		log.Error(err, "Can't load caps for current process")
		return
	}
	caps.Set(EFFECTIVE, CAP_NET_RAW) // NET_ADMIN is NOT sufficent; it is NOT a superset of NET_RAW
	// a nil error doesn't indicate caps were sucessfully applied
	err = caps.Apply(CAPS) // have to give (all) CAPS, not just EFFECTIVE. I guess the kernel wants to see you present it in your permitted set too (seems weird that it would trust you)
	if err != nil {
		log.Error(err, "Can't update effective caps set for current process. Are the right permitted caps present on the executable?")
		return
	}
	err = caps.Load()
	if err != nil {
		log.Error(err, "Can't re-load caps for current process")
		return
	}
	log.Info("Caps", "new", caps.String())
}

func DropNetRaw(log logr.Logger) {
	log.Info("Dropping CAP_NET_RAW")

	log.WithValues("pid (self)", os.Getpid())
	caps, err := NewPid2(0)
	if err != nil {
		log.Error(err, "Can't construct caps object for current process")
		return
	}
	err = caps.Load()
	log.Info("Caps", "original", caps.String())
	if err != nil {
		log.Error(err, "Can't load caps for current process")
		return
	}
	caps.Unset(EFFECTIVE, CAP_NET_RAW)
	// a nil error doesn't indicate caps were sucessfully applied
	err = caps.Apply(CAPS)
	if err != nil {
		log.Error(err, "Can't update effective caps set for current process")
		return
	}
	err = caps.Load()
	if err != nil {
		log.Error(err, "Can't re-load caps for current process")
		return
	}
	log.Info("Caps", "new", caps.String())
}

func logFileCaps(log logr.Logger) {
	exe, err := os.Executable()
	if err != nil {
		log.Error(err, "Can't get current executable path")
		return
	}
	log = log.WithValues("path", exe)

	caps, err := NewFile(exe)
	if err != nil {
		log.Error(err, "Can't construct caps object for running binary")
		return
	}
	err = caps.Load()
	if err != nil {
		log.Error(err, "Can't load caps for running binary file")
		return
	}
	log.Info("Caps", "filesystem", caps.String())
}
