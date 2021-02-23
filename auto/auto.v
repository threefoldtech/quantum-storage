module main

import os
import net.http
import json

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

fn download(zflist string, zdbfs string) {
	zflist_bin := base(zflist)
	zdbfs_file := base(zdbfs)

	println("[+] downloading: " + zflist_bin)
	http.download_file(zflist, zflist_bin) or { eprintln(err) }
	os.chmod(zflist_bin, 0o775)

	println("[+] downloading: " + zdbfs_file)
	http.download_file(zdbfs, zdbfs_file) or { eprintln(err) }
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

	return true
}

fn extract(rootdir string, zflist string, zdbfs string) {
	zflist_bin := base(zflist)
	zdbfs_file := base(zdbfs)

	os.setenv("ZFLIST_MNT", os.join_path(rootdir, "var", "tmp", "flistmnt"), true)
	os.setenv("ZFLIST_JSON", "1", true)
	os.setenv("ZFLIST_PROGRESS", "1", true)

	zflist_run("./" + zflist_bin, ["open", zdbfs_file], false)
	zflist_run("./" + zflist_bin, ["metadata", "backend", "--host", "hub.grid.tf", "--port", "9900"], false)

	files := ["/bin/etcd", "/bin/zdb", "/bin/zdbfs", "/bin/zstor-v2", "/lib/libfuse3.so.3.9.0", "/var/lib/zdb-hook.sh"]

	// FIXME: add hook extraction

	for file in files {
		print("[+] extracting: " + file + " ")

		target := os.join_path(rootdir, file)

		zflist_run("./" + zflist_bin, ["get", file, target], true)
		os.chmod(target, 0o775)
	}

	zflist_run("./" + zflist_bin, ["close"], false)
}

fn databases(rootdir string) {
	//
	// 0-db
	//
	println("[+] starting 0-db local cache")

	// FIXME: check for already running

	mut zdb := os.new_process(rootdir + "/bin/zdb")
	zargs := [
		"--index", rootdir + "/var/tmp/zdb/index",
		"--data", rootdir + "/var/tmp/zdb/data",
		"--hook", rootdir + "/var/lib/zdb-hook.sh",
		"--mode", "seq",
		"--background"
	]

	zenvs := map{
		"ZDBFS_PREFIX": rootdir,
	}

	zdb.set_args(zargs)
	zdb.set_environment(zenvs)
	zdb.set_redirect_stdio()
	zdb.run()
	zdb.wait()

	//
	// etcd
	//
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

	// FIXME: set environment for LD PATH

	ps.set_args(args)
	ps.run()
	ps.wait()
}

fn main() {
	// resources url
	zflist := "https://github.com/threefoldtech/0-flist/releases/download/v2.0.1/zflist-2.0.1-amd64-linux-gnu"
	zdbfs := "https://hub.grid.tf/maxux42.3bot/zdbfs-0.1.2-linux-gnu.flist"

	home := os.getenv("HOME")
	rootdir := os.join_path(home, ".threefold")
	println("[+] prefix: " + rootdir)

	prefix(rootdir)
	os.chdir(rootdir)

	download(zflist, zdbfs)
	extract(rootdir, zflist, zdbfs)

	databases(rootdir)
	filesystem(rootdir)
}
