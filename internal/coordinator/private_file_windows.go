//go:build windows

package coordinator

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows ignores POSIX 0600 on NTFS. Replace the inherited DACL with grants
// solely for the owner, LocalSystem and BUILTIN Administrators. The DACL is
// protected so a parent directory cannot later re-introduce Everyone/Users.
func securePrivateFile(path string) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil {
		return windows.ERROR_INVALID_OWNER
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"D:P(A;;FA;;;" + owner.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil {
		if err != nil {
			return err
		}
		return windows.ERROR_INVALID_ACL
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil)
}

// Read only a protected DACL whose allow entries grant access solely to the
// file owner and Windows administrator/system principals. Fail closed if the
// ACL cannot be examined, is unprotected, or admits an unknown SID.
func privateFileReadable(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return false
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return false
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		return false
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil { return false }
	admins, err := windows.StringToSid("S-1-5-32-544")
	if err != nil { return false }
	hasOwner := false
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil ||
			ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return false
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(owner) {
			hasOwner = true
		} else if !sid.Equals(system) && !sid.Equals(admins) {
			return false
		}
	}
	return hasOwner
}
