//go:build linux

package encierro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/hoomdev/hoomai/internal/procesos"
)

// marcaPaso is the variable that tells a hoom started inside the encierro
// that it is the step (Paso): bwrap sets it for the step only, to the level
// of the encierro, and the step takes it out before it executes the command.
// With NivelCompleto the step applies Landlock; with any level it makes the
// command the leader of a session of its own, with no controlling terminal.
const marcaPaso = "HOOM_ENCIERRO_PASO"

// topeSonda bounds the probe's bwrap; topeInfo how long Arrancar waits for
// bwrap to report the pid of its init; topeFin how long Liberar waits for the
// encierro to be gone once its init got its SIGKILL.
const (
	topeSonda = 10 * time.Second
	topeInfo  = 10 * time.Second
	topeFin   = 5 * time.Second
)

// banderasBwrap are the namespaces and the rest the probe and every enclosed
// launch ask bwrap for: its own PID, user and mount namespaces (the mount one
// comes with the user one), a new session without a controlling terminal,
// and death with the process that launched it. bwrap gives no_new_privs and
// drops every capability by itself.
var banderasBwrap = []string{"--unshare-user", "--unshare-pid", "--new-session", "--die-with-parent"}

// sondear is the probe of this Linux: bwrap with the minimal view and a
// command that ends, and Landlock's ABI.
func sondear() Sonda {
	s := Sonda{LandlockABI: landlockABI()}
	b, err := exec.LookPath("bwrap")
	if err != nil {
		return s
	}
	s.Bwrap = true
	ctx, cancel := context.WithTimeout(context.Background(), topeSonda)
	defer cancel()
	args := append([]string{"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc"}, banderasBwrap...)
	args = append(args, "--", "true")
	cmd := exec.CommandContext(ctx, b, args...)
	cmd.WaitDelay = time.Second
	var salida bytes.Buffer
	cmd.Stdout, cmd.Stderr = &salida, &salida
	if err := cmd.Run(); err != nil {
		s.Salida = strings.TrimSpace(salida.String())
		if s.Salida == "" {
			s.Salida = "bwrap: " + err.Error()
		}
		return s
	}
	s.Espacio = true
	return s
}

// landlockABI asks Landlock its ABI: 0 when there is no Landlock.
func landlockABI() int {
	abi, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if e != 0 {
		return 0
	}
	return int(abi)
}

// restringir forbids this thread, and what it executes, to connect to an
// abstract unix socket outside its Landlock domain.
func restringir() error {
	attr := unix.LandlockRulesetAttr{Scoped: unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET}
	fd, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if e != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", e)
	}
	defer unix.Close(int(fd))
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	if _, _, e := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0); e != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", e)
	}
	return nil
}

// envuelto is what Envolver left for Arrancar and Liberar of one command
// prepared with bwrap.
type envuelto struct {
	// info is the read end of bwrap's --info-fd; infoW its write end, which
	// goes to bwrap; vacios are the empty files --ro-bind-data reads.
	info, infoW *os.File
	vacios      []*os.File
	mu          sync.Mutex
	// pid is the outside pid of bwrap's init (pid 1 inside); 0 until bwrap
	// reports it. pidfd is a pidfd of it, -1 when there is none.
	pid, pidfd int
}

// pidInit is the outside pid of bwrap's init, 0 when unknown.
func (e *envuelto) pidInit() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pid
}

// matarInit sends SIGKILL to bwrap's init, which ends the whole PID
// namespace. It does not wait, and it never signals a pid that is not that
// init any more (it goes through the pidfd).
func (e *envuelto) matarInit() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pidfd >= 0 {
		_ = unix.PidfdSendSignal(e.pidfd, unix.SIGKILL, nil, 0)
	}
}

// terminar ends the encierro, if anything of it is left, and waits until it
// is gone: when bwrap's init ends, the kernel has already killed everything
// else inside.
func (e *envuelto) terminar() {
	e.matarInit()
	e.esperarFin(time.Now().Add(topeFin))
}

// cerrar closes what Envolver opened.
func (e *envuelto) cerrar() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, f := range append([]*os.File{e.info, e.infoW}, e.vacios...) {
		if f != nil {
			f.Close()
		}
	}
	e.info, e.infoW, e.vacios = nil, nil, nil
	if e.pidfd >= 0 {
		unix.Close(e.pidfd)
		e.pidfd = -1
	}
}

// envolver is Envolver with mechanism bwrap.
func envolver(cmd *exec.Cmd, p Plan) error {
	if cmd.Err != nil || cmd.Process != nil {
		return nil // Start reports its own error; a started one is not touched
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return errSinBwrap
	}
	v := p.Vista
	if v.Cache == "" {
		return fmt.Errorf("encierro: la vista no tiene cache")
	}
	for _, d := range dirsDeEntorno(v) {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("encierro: no se pudo crear la cache %s: %w", d, err)
		}
	}
	dir := cmd.Dir
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(dir) {
		if dir, err = filepath.Abs(dir); err != nil {
			return err
		}
	}
	env := Entorno(cmd.Environ(), v)
	home := valorDe(env, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	e := &envuelto{pidfd: -1}
	ok := false
	defer func() {
		if !ok {
			e.cerrar()
		}
	}()
	e.info, e.infoW, err = os.Pipe()
	if err != nil {
		return err
	}
	extra := append([]*os.File(nil), cmd.ExtraFiles...)
	fdDe := func(f *os.File) string {
		extra = append(extra, f)
		return strconv.Itoa(2 + len(extra))
	}
	vacio := func() (string, error) {
		f, err := os.Open(os.DevNull)
		if err != nil {
			return "", err
		}
		e.vacios = append(e.vacios, f)
		return fdDe(f), nil
	}

	args := []string{"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc"}
	for _, m := range montajes(v) {
		if !existe(m.ruta) {
			continue
		}
		if m.escribe {
			args = append(args, "--bind", m.ruta, m.ruta)
		} else {
			args = append(args, "--ro-bind", m.ruta, m.ruta)
		}
	}
	// the known secrets: an empty directory or an empty file, read-only
	for _, s := range SecretosDirs {
		if d := expandir(s, home); d != "" && esDir(d) {
			args = append(args, "--tmpfs", d, "--remount-ro", d)
		}
	}
	tapar := func(p string) error {
		fi, err := os.Lstat(p)
		if err != nil {
			return nil // what is not there is not covered
		}
		if fi.IsDir() {
			args = append(args, "--tmpfs", p, "--remount-ro", p)
			return nil
		}
		fd, err := vacio()
		if err != nil {
			return err
		}
		args = append(args, "--ro-bind-data", fd, p)
		return nil
	}
	for _, s := range SecretosArchivos {
		if f := expandir(s, home); f != "" && existe(f) && !esDir(f) {
			if err := tapar(f); err != nil {
				return err
			}
		}
	}
	// the sockets that launch processes: an empty directory (or file)
	for _, p := range socketsTapados() {
		if err := tapar(p); err != nil {
			return err
		}
	}
	args = append(args, banderasBwrap...)
	args = append(args, "--info-fd", fdDe(e.infoW), "--chdir", dir)
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("encierro: no se sabe donde esta hoom para su paso: %w", err)
	}
	argv := cmd.Args
	if len(argv) == 0 {
		argv = []string{cmd.Path}
	}
	nivel := NivelParcial
	if p.Info.Nivel == NivelCompleto {
		nivel = NivelCompleto
	}
	args = append(args, "--setenv", marcaPaso, nivel, "--", exe, cmd.Path)
	args = append(args, argv...)
	cmd.Path = bwrap
	cmd.Args = append([]string{bwrap}, args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.ExtraFiles = extra
	ok = true
	envueltos.Lock()
	envueltos.por[cmd] = e
	envueltos.Unlock()
	return nil
}

// socketsTapados are the places of the sockets that launch processes the
// encierro covers, the ones that exist: RunUser(), /run/docker.sock,
// /run/podman, and in /tmp tmux-<uid>, .X11-unix, .ICE-unix and the ssh-*
// of this user.
func socketsTapados() []string {
	uid := os.Getuid()
	var out []string
	for _, p := range []string{RunUser(), "/run/docker.sock", "/run/podman", "/tmp/tmux-" + strconv.Itoa(uid), "/tmp/.X11-unix", "/tmp/.ICE-unix"} {
		if existe(p) {
			out = append(out, p)
		}
	}
	if es, err := os.ReadDir("/tmp"); err == nil {
		for _, e := range es {
			if !strings.HasPrefix(e.Name(), "ssh-") {
				continue
			}
			p := filepath.Join("/tmp", e.Name())
			fi, err := os.Lstat(p)
			if err != nil {
				continue
			}
			if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) == uid {
				out = append(out, p)
			}
		}
	}
	return out
}

// arrancarEnvuelto starts cmd, prepared by Envolver, in a process group of
// its own registered with procesos, whose polite signal goes to the group of
// the main process inside (the group of bwrap's init) and whose end kills
// bwrap's init; then it waits for bwrap to report that init.
func arrancarEnvuelto(cmd *exec.Cmd, gracia time.Duration, e *envuelto, alCancelar func()) error {
	term := func() bool {
		pid := e.pidInit()
		if pid <= 0 {
			return false
		}
		ok := false
		// the main process inside leads a group of its own (the step's
		// setsid); before that, it is in its init's group
		if c := comandoDe(pid); c > 0 && syscall.Kill(-c, syscall.SIGTERM) == nil {
			ok = true
		}
		if syscall.Kill(-pid, syscall.SIGTERM) == nil {
			ok = true
		}
		return ok
	}
	err := procesos.ArrancarCon(cmd, gracia, procesos.Terminacion{Term: term, Matar: e.matarInit, AlCancelar: alCancelar})
	e.mu.Lock()
	if e.infoW != nil {
		e.infoW.Close() // bwrap keeps its copy
		e.infoW = nil
	}
	for _, f := range e.vacios {
		f.Close()
	}
	e.vacios = nil
	info := e.info
	e.mu.Unlock()
	if err != nil {
		return err
	}
	pid := leerInfo(info, cmd.Process.Pid)
	if pid <= 0 {
		return nil // bwrap ended (or failed) before saying: its group is all there is
	}
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil || padreDe(pid) != cmd.Process.Pid {
		if err == nil {
			unix.Close(fd)
		}
		return nil
	}
	e.mu.Lock()
	e.pid, e.pidfd = pid, fd
	e.mu.Unlock()
	return nil
}

// leerInfo reads bwrap's --info-fd: the outside pid of its init
// (`child-pid`), 0 when bwrap closed it without saying or took too long.
func leerInfo(info *os.File, _ int) int {
	if info == nil {
		return 0
	}
	_ = info.SetReadDeadline(time.Now().Add(topeInfo))
	var datos struct {
		ChildPid int `json:"child-pid"`
	}
	if err := json.NewDecoder(info).Decode(&datos); err != nil {
		return 0
	}
	return datos.ChildPid
}

// comandoDe is the outside pid of the main process of the encierro whose
// init is init: the child of init that is pid 2 inside (NSpid), the first
// process bwrap's init starts. 0 when it is not there.
func comandoDe(init int) int {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(init) + "/task/" + strconv.Itoa(init) + "/children")
	if err != nil {
		return 0
	}
	for _, c := range strings.Fields(string(raw)) {
		st, err := os.ReadFile("/proc/" + c + "/status")
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(st), "\n") {
			if rest, ok := strings.CutPrefix(l, "NSpid:"); ok {
				if f := strings.Fields(rest); len(f) > 1 && f[len(f)-1] == "2" {
					pid, _ := strconv.Atoi(c)
					return pid
				}
			}
		}
	}
	return 0
}

// esperarFin waits until bwrap's init is gone or hasta passes.
func (e *envuelto) esperarFin(hasta time.Time) {
	e.mu.Lock()
	fd := e.pidfd
	e.mu.Unlock()
	if fd < 0 {
		return
	}
	for time.Now().Before(hasta) {
		pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(pfd, int(time.Until(hasta)/time.Millisecond)+1)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n > 0 {
			return
		}
	}
}

// padreDe is the parent pid of pid, from /proc; 0 when it cannot say.
func padreDe(pid int) int {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	i := bytes.LastIndexByte(raw, ')')
	if i < 0 {
		return 0
	}
	campos := strings.Fields(string(raw[i+1:]))
	if len(campos) < 2 {
		return 0
	}
	p, _ := strconv.Atoi(campos[1])
	return p
}

// valorDe is the value of the variable k in env, "" when it is not there.
func valorDe(env []string, k string) string {
	v := ""
	for _, kv := range env {
		if kk, vv, ok := strings.Cut(kv, "="); ok && kk == k {
			v = vv
		}
	}
	return v
}

// existe says whether something is at p (not followed).
func existe(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// esDir says whether p is a directory (followed).
func esDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// dirsDeEntorno are the directories Entorno points at inside v.Cache, which
// Envolver creates.
func dirsDeEntorno(v Vista) []string {
	out := []string{v.Cache, filepath.Join(v.Cache, "TMPDIR"), filepath.Join(v.Cache, "XDG_CACHE_HOME")}
	for _, k := range v.VarsCache {
		out = append(out, filepath.Join(v.Cache, k))
	}
	return out
}

// montaje is one bind of the view: writable or read-only.
type montaje struct {
	ruta    string
	escribe bool
}

// montajes are the binds of v in the order bwrap applies them: the
// shallower first, so what is inside a path keeps its own mode (the git
// material read-only inside the writable common directory, a worktree's own
// administration writable inside the read-only worktrees/); at the same
// depth the writable ones first, so a path in both lists stays read-only.
func montajes(v Vista) []montaje {
	var out []montaje
	for _, w := range append(append([]string(nil), v.Escribibles...), v.Cache) {
		out = append(out, montaje{ruta: w, escribe: true})
	}
	for _, r := range v.SoloLectura {
		out = append(out, montaje{ruta: r})
	}
	sep := string(filepath.Separator)
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := strings.Count(filepath.Clean(out[i].ruta), sep), strings.Count(filepath.Clean(out[j].ruta), sep)
		if di != dj {
			return di < dj
		}
		return out[i].escribe && !out[j].escribe
	})
	return out
}
