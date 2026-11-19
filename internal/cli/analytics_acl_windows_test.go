package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestAnalyticsInstallationIDWindowsACL(t *testing.T) {
	a := analyticsTestApp(t, "")
	if a.analyticsIdentity() == "" {
		t.Fatal("could not create installation ID")
	}
	path := filepath.Join(a.env["AGORA_HOME"], "analytics-id")
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("installation ID must have a DACL, got %v", err)
	}
	var computerName [256]uint16
	size := uint32(len(computerName))
	if err := windows.GetComputerName(&computerName[0], &size); err != nil {
		t.Fatal(err)
	}
	localDomain := windows.UTF16ToString(computerName[:size])
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("unexpected installation ID ACL entry type %d", ace.Header.AceType)
		}
		if ace.Mask&(windows.FILE_READ_DATA|windows.GENERIC_READ|windows.GENERIC_ALL) == 0 {
			continue
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if principal.Equals(owner) || principal.IsWellKnown(windows.WinLocalSystemSid) || principal.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			continue
		}
		// The local built-in Administrator has a machine-specific SID with RID 500,
		// separate from the well-known Administrators group SID.
		if principal.SubAuthorityCount() == 5 && principal.SubAuthority(4) == 500 {
			_, domain, accountType, err := principal.LookupAccount("")
			if err == nil && accountType == windows.SidTypeUser && strings.EqualFold(domain, localDomain) {
				continue
			}
		}
		t.Errorf("installation ID is readable by unexpected principal %s", principal)
	}
}
