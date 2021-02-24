module main

import os
import net
import net.http
import json

struct Resources {
	zflist string  // zflist binary url
	zdbfs string   // zdbfs flist url
	zstor string   // zstor sample config url
}

struct ZError {
	function string
	message string
}

struct Response {

}

struct ZResponse {
	status string
	success bool
	message string
	current int
	total int
	error ZError
	response Response
}

fn mount_check(rootdir string) bool {
	check := os.exec("mountpoint " + rootdir + "/mnt/zdbfs") or { return true }

	// target is a mountpoint
	if check.exit_code == 0 {
		return false
	}

	return true
}

fn prefix(dir string) {
	dirs := [
		dir,
		os.join_path(dir, "bin"),
		os.join_path(dir, "etc"),
		os.join_path(dir, "lib"),
		os.join_path(dir, "var"),
		os.join_path(dir, "var", "cache"),
		os.join_path(dir, "var", "tmp"),
		os.join_path(dir, "var", "lib"),
		os.join_path(dir, "mnt"),
		os.join_path(dir, "mnt", "zdbfs")
	]

	println("[+] creating root directories")
	for path in dirs {
		// println(path)
		os.mkdir(path) or { continue }
	}
}

fn download(resources Resources) {
	zflist_bin := base(resources.zflist)
	zdbfs_file := base(resources.zdbfs)
	zstor_file := base(resources.zstor)

	println("[+] downloading: " + zflist_bin)
	http.download_file(resources.zflist, zflist_bin) or { eprintln(err) }
	os.chmod(zflist_bin, 0o775)

	println("[+] downloading: " + zdbfs_file)
	http.download_file(resources.zdbfs, zdbfs_file) or { eprintln(err) }

	println("[+] downloading: " + zstor_file)
	http.download_file(resources.zstor, zstor_file) or { eprintln(err) }
}

fn zflist_json(line string, progress bool) {
	if line.trim(" \n") == "" {
		return
	}

	data := json.decode(ZResponse, line) or { eprintln("nope") eprintln(err) exit(1) }

	// success response
	if data.success == true {
		if progress == true {
			// progression ended
			println(" ok")
		}

		return
	}

	// progression step
	if data.status == "progress" {
		// dot progression
		print(".")
		return
	}
}

fn zflist_run(binary string, args []string, progress bool) bool {
	mut ps := os.new_process(binary)
	ps.set_args(args)
	ps.set_redirect_stdio()
	ps.run()

	for ps.is_alive() {
		line := ps.stdout_read()
		zflist_json(line, progress)
	}

	if ps.code == 1 {
		println("(failed)")
	}

	return true
}

fn extract(rootdir string, resources Resources) {
	zflist_bin := base(resources.zflist)
	zdbfs_file := base(resources.zdbfs)
	zstor_file := base(resources.zstor)

	println("[+] installing: zstor-v2 configuration")
	os.cp(zstor_file, rootdir + "/etc/zstor-default.toml") or { eprintln(err) }

	os.setenv("ZFLIST_MNT", rootdir + "/var/tmp/flistmnt", true)
	os.setenv("ZFLIST_JSON", "1", true)
	os.setenv("ZFLIST_PROGRESS", "1", true)

	zflist_run("./" + zflist_bin, ["open", zdbfs_file], false)
	zflist_run("./" + zflist_bin, ["metadata", "backend", "--host", "hub.grid.tf", "--port", "9900"], false)

	files := [
		"/bin/etcd",
		"/bin/zdb",
		"/bin/zdbfs",
		"/bin/zstor-v2",
		"/lib/libfuse3.so.3.9.0",
		"/var/lib/zdb-hook.sh"
	]

	for file in files {
		print("[+] extracting: " + file + " ")

		target := os.join_path(rootdir, file)

		zflist_run("./" + zflist_bin, ["get", file, target], true)
		os.chmod(target, 0o775)
	}

	zflist_run("./" + zflist_bin, ["close"], false)

	// symlink library version
	os.chdir(os.join_path(rootdir, "lib"))
	os.symlink("libfuse3.so.3.9.0", "libfuse3.so.3") or { return }
	os.chdir(rootdir)
}

fn zdb_precheck(rootdir string) bool {
	println("[+] checking for a local 0-db")

	mut conn := net.dial_tcp("127.0.0.1:9900") or {
		/* this is ignored, bug filled */
		return false
	}

	conn.write_str("#1") or {
		if err == "net: socket error: 111" {
			// connection refused
			return false
		}

		// we return false anyway on error
		return false
	}

	return true
}

fn etcd_precheck(rootdir string) bool {
	println("[+] checking for local etcd server")

	http.get("http://127.0.0.1:2379") or { return false }
	println("[+] local etcd already available")

	return true
}

fn zdb_init(rootdir string) {
	println("[+] starting 0-db local cache")

	mut zdb := os.new_process(rootdir + "/bin/zdb")
	zargs := [
		"--index", rootdir + "/var/tmp/zdb/index",
		"--data", rootdir + "/var/tmp/zdb/data",
		"--hook", rootdir + "/var/lib/zdb-hook.sh",
		"--mode", "seq",
		"--background"
	]

	// set zdb-hook prefix environment
	zenvs := map{
		"ZDBFS_PREFIX": rootdir,
	}

	zdb.set_args(zargs)
	zdb.set_environment(zenvs)
	zdb.set_redirect_stdio()
	zdb.run()
	zdb.wait()
}

fn etcd_init(rootdir string) {
	println("[+] starting local etcd server")

	mut etcd := os.new_process(rootdir + "/bin/etcd")
	eargs := [
		"--data-dir", rootdir + "/var/tmp/etcd"
	]

	etcd.set_args(eargs)
	etcd.set_redirect_stdio()
	etcd.run()
}

fn filesystem(rootdir string) {
	println("[+] starting planetary filesystem")

	mut ps := os.new_process(rootdir + "/bin/zdbfs")
	args := [
		"-o", "autons",
		"-o", "background",
		rootdir + "/mnt/zdbfs"
	]

	envs := map{
		"LD_LIBRARY_PATH": rootdir + "/lib"
	}

	ps.set_args(args)
	ps.set_environment(envs)
	ps.run()
	ps.wait()
}

fn main() {
	resources := Resources{
		zflist: "https://github.com/threefoldtech/0-flist/releases/download/v2.0.1/zflist-2.0.1-amd64-linux-gnu",
		zdbfs: "https://hub.grid.tf/maxux42.3bot/zdbfs-0.1.2-linux-gnu.flist",
		zstor: "https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/zstor-sample-ipv4.toml",
	}

	home := os.getenv("HOME")
	rootdir := os.join_path(home, ".threefold")
	println("[+] prefix: " + rootdir)

	if mount_check(rootdir) == false {
		println("[-] planetary filesystem already mounted")
		return
	}

	prefix(rootdir)
	os.chdir(rootdir)

	download(resources)
	extract(rootdir, resources)

	if zdb_precheck(rootdir) == false {
		zdb_init(rootdir)
	}

	if etcd_precheck(rootdir) == false {
		etcd_init(rootdir)
	}

	filesystem(rootdir)
}
