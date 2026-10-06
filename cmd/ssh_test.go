package cmd

import (
	"os/exec"
	"reflect"
	"strconv"
	"testing"
)

// The notice OpenSSH logs at ERROR level when a known host's key changed.
// With StrictHostKeyChecking=no the connection still goes ahead, so this
// alone must not count as a failed connection.
const hostKeyChangedNotice = "@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\r\n" +
	"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\r\n" +
	"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\r\n" +
	"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!\r\n" +
	"Someone could be eavesdropping on you right now (man-in-the-middle attack)!\r\n" +
	"It is also possible that a host key has just been changed.\r\n" +
	"The fingerprint for the ED25519 key sent by the remote host is\n" +
	"SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG.\r\n" +
	"Please contact your system administrator.\r\n" +
	"Add correct host key in /home/core/.ssh/known_hosts to get rid of this message.\r\n" +
	"Offending ECDSA key in /home/core/.ssh/known_hosts:42\r\n" +
	"  remove with:\r\n" +
	"  ssh-keygen -f \"/home/core/.ssh/known_hosts\" -R \"[203.0.113.5]:2222\"\r\n" +
	"Password authentication is disabled to avoid man-in-the-middle attacks.\r\n" +
	"Keyboard-interactive authentication is disabled to avoid man-in-the-middle attacks.\r\n" +
	"UpdateHostkeys is disabled because the host key is not trusted.\r\n"

func TestSSHLogErrors(t *testing.T) {
	cases := []struct {
		name string
		log  string
		want []string
	}{
		{"empty log", "", nil},
		{"host key changed only", hostKeyChangedNotice, nil},
		{"refused", "ssh: connect to host 203.0.113.5 port 2222: Connection refused\r\n",
			[]string{"ssh: connect to host 203.0.113.5 port 2222: Connection refused"}},
		{"host key changed then auth failed", hostKeyChangedNotice + "user@203.0.113.5: Permission denied (publickey).\r\n",
			[]string{"user@203.0.113.5: Permission denied (publickey)."}},
	}
	for _, c := range cases {
		if got := sshLogErrors(c.log); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: sshLogErrors() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestExitStatus(t *testing.T) {
	if got := exitStatus(nil); got != 0 {
		t.Errorf("exitStatus(nil) = %d, want 0", got)
	}
	for _, want := range []int{1, 5, 255} {
		err := exec.Command("bash", "-c", "exit "+strconv.Itoa(want)).Run()
		if got := exitStatus(err); got != want {
			t.Errorf("exitStatus(exit %d) = %d, want %d", want, got, want)
		}
	}
	if got := exitStatus(exec.Command("/nonexistent/binary").Run()); got != 1 {
		t.Errorf("exitStatus(start failure) = %d, want 1", got)
	}
}
