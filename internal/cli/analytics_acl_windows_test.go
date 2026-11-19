package cli

import (
	"path/filepath"
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
		if !principal.Equals(owner) && !principal.IsWellKnown(windows.WinLocalSystemSid) && !principal.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			t.Errorf("installation ID is readable by unexpected principal %s", principal)
		}
	}
}
