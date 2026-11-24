//go:build windows

package windows

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/developerasaad/winprep/internal/platform"
	"golang.org/x/sys/windows"
)

// LocalAccounts enumerates the machine's local user accounts through the
// documented SAM management APIs (netapi32): NetUserEnum for the names and
// NetUserGetInfo level 4 for each account's SID and flags. This is a direct
// API query, so nothing is parsed from localized command output and no
// external process is spawned.
//
// NetUserGetInfo at level 4 requires administrative rights, which the
// supported workflow already demands; when it fails, the handover treats the
// account list as unverifiable rather than empty.
func (s *System) LocalAccounts(ctx context.Context) ([]platform.LocalAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	names, err := enumLocalUserNames()
	if err != nil {
		return nil, err
	}

	accounts := make([]platform.LocalAccount, 0, len(names))
	for _, name := range names {
		acct, err := localAccountInfo(name)
		if err != nil {
			return nil, fmt.Errorf("reading account %q: %w", name, err)
		}
		accounts = append(accounts, acct)
	}
	return accounts, nil
}

var (
	netapi32             = windows.NewLazySystemDLL("netapi32.dll")
	procNetUserEnum      = netapi32.NewProc("NetUserEnum")
	procNetUserGetInfo   = netapi32.NewProc("NetUserGetInfo")
	procNetApiBufferFree = netapi32.NewProc("NetApiBufferFree")
)

const (
	filterNormalAccount = 0x0002 // FILTER_NORMAL_ACCOUNT
	maxPreferredLength  = 0xFFFFFFFF
	nerrSuccess         = 0
	errMoreData         = 234 // ERROR_MORE_DATA
	ufAccountDisable    = 0x0002
	// enumPageLimit bounds the NetUserEnum resume loop so a corrupt or
	// hostile SAM cannot spin the caller forever.
	enumPageLimit = 100
)

// userInfo0 mirrors USER_INFO_0: a pointer to the account name.
type userInfo0 struct {
	Name *uint16
}

// userInfo4 mirrors USER_INFO_4. Only the fields WinPrep reads are named; the
// remaining members keep the documented layout so the struct can be used
// directly as the buffer returned by NetUserGetInfo.
type userInfo4 struct {
	Name            *uint16
	Password        *uint16
	PasswordAge     uint32
	Priv            uint32
	HomeDir         *uint16
	Comment         *uint16
	Flags           uint32
	ScriptPath      *uint16
	AuthFlags       uint32
	FullName        *uint16
	UsrComment      *uint16
	Parms           *uint16
	Workstations    *uint16
	LastLogon       uint32
	LastLogoff      uint32
	AcctExpires     uint32
	MaxStorage      uint32
	UnitsPerWeek    uint32
	LogonHours      *byte
	BadPwCount      uint32
	NumLogons       uint32
	LogonServer     *uint16
	Country         uint32
	CodePage        uint32
	UserSid         *windows.SID
	PrimaryGroupID  uint32
	Profile         *uint16
	HomeDirDrive    *uint16
	PasswordExpired uint32
}

func netApiBufferFree(buf uintptr) {
	_, _, _ = procNetApiBufferFree.Call(buf)
}

// enumLocalUserNames returns the names of the normal local user accounts.
// NetUserEnum pages results through the resume handle when the buffer cannot
// hold everything at once; with MAX_PREFERRED_LENGTH a small local SAM is
// returned in a single call.
func enumLocalUserNames() ([]string, error) {
	var names []string
	var resume uint32
	for page := 0; ; page++ {
		var buf *userInfo0
		var read, total uint32
		rc, _, _ := procNetUserEnum.Call(
			0, // NULL server name: the local machine
			0, // level 0: names only
			filterNormalAccount,
			uintptr(unsafe.Pointer(&buf)),
			maxPreferredLength,
			uintptr(unsafe.Pointer(&read)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&resume)),
		)
		if rc != nerrSuccess && rc != errMoreData {
			return nil, fmt.Errorf("NetUserEnum failed with code %d", rc)
		}
		if buf != nil {
			entries := unsafe.Slice(buf, read)
			for i := range entries {
				if entries[i].Name != nil {
					names = append(names, windows.UTF16PtrToString(entries[i].Name))
				}
			}
			netApiBufferFree(uintptr(unsafe.Pointer(buf)))
		}
		if rc != errMoreData || (page+1) >= enumPageLimit {
			break
		}
	}
	return names, nil
}

// localAccountInfo fetches one account's SID and disabled flag.
func localAccountInfo(name string) (platform.LocalAccount, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return platform.LocalAccount{}, err
	}
	var buf *userInfo4
	rc, _, _ := procNetUserGetInfo.Call(
		0,
		uintptr(unsafe.Pointer(namePtr)),
		4,
		uintptr(unsafe.Pointer(&buf)),
	)
	if rc != nerrSuccess {
		return platform.LocalAccount{}, fmt.Errorf("NetUserGetInfo failed with code %d", rc)
	}
	if buf == nil {
		return platform.LocalAccount{}, fmt.Errorf("NetUserGetInfo returned no data")
	}
	defer netApiBufferFree(uintptr(unsafe.Pointer(buf)))

	acct := platform.LocalAccount{Name: name}
	if buf.UserSid != nil {
		var sidStr *uint16
		if err := windows.ConvertSidToStringSid(buf.UserSid, &sidStr); err != nil {
			return platform.LocalAccount{}, fmt.Errorf("converting SID: %w", err)
		}
		acct.SID = windows.UTF16PtrToString(sidStr)
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(sidStr)))
	}
	acct.RID = sidRID(acct.SID)
	acct.Disabled = buf.Flags&ufAccountDisable != 0
	return acct, nil
}

// sidRID extracts the relative identifier from an SID string such as
// S-1-5-21-1000-2000-3000-1013. Unparseable SIDs return 0, which is never
// >= 1000 and therefore never flags an account as extra.
func sidRID(sid string) uint32 {
	var rid uint32
	last := -1
	for j := 0; j < len(sid); j++ {
		if sid[j] == '-' {
			last = j
		}
	}
	if last < 0 || last == len(sid)-1 {
		return 0
	}
	for _, c := range sid[last+1:] {
		if c < '0' || c > '9' {
			return 0
		}
		rid = rid*10 + uint32(c-'0')
	}
	return rid
}
