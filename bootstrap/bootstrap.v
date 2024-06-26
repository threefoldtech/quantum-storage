module main

import os
import net
import net.http
import json
import regex

struct Resources {
	zflist string  // zflist binary url
	zdbfs string   // zdbfs flist url

mut:
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
	check := os.execute("mountpoint " + rootdir + "/mnt/zdbfs")

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
	zflist_bin := os.base(resources.zflist)
	zdbfs_file := os.base(resources.zdbfs)
	zstor_file := os.base(resources.zstor)

	println("[+] downloading: " + zflist_bin)
	http.download_file(resources.zflist, zflist_bin) or { eprintln(err) }
	os.chmod(zflist_bin, 0o775) or { eprintln(err) }

	println("[+] downloading: " + zdbfs_file)
	http.download_file(resources.zdbfs, zdbfs_file) or { eprintln(err) }

	// download zstor config if it's an url
	if resources.zstor.substr(0, 4) == "http" {
		println("[+] downloading: " + zstor_file)
		http.download_file(resources.zstor, zstor_file) or { eprintln(err) }

	} else {
		// copy file location provided
		mut source := resources.zstor

		if source.substr(0, 1) != "/" {
			source = os.resource_abs_path(resources.zstor)
		}

		println("[+] copying: " + source)
		os.cp(source, zstor_file) or { eprintln(err) exit(1) }
	}
}

fn zflist_json(buffer string, progress bool) {
	if buffer.trim(" \n") == "" {
		return
	}

	lines := buffer.split("\n")

	for line in lines {
		if line == "" {
			return
		}

		data := json.decode(ZResponse, line) or { eprintln("zflist error") eprintln(err) exit(1) }

		// success response
		if data.success == true {
			if progress == true {
				// progression ended
				println(" ok")
			}

			continue
		}

		// progression step
		if data.status == "progress" {
			// dot progression
			print(".")
			continue
		}
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
	zflist_bin := os.base(resources.zflist)
	zdbfs_file := os.base(resources.zdbfs)
	zstor_file := os.base(resources.zstor)

	println("[+] installing: zstor-v2 configuration")
	os.cp(zstor_file, rootdir + "/etc/zstor-default.toml") or { eprintln(err) }

	os.setenv("ZFLIST_MNT", rootdir + "/var/tmp/flistmnt", true)
	os.setenv("ZFLIST_JSON", "1", true)
	os.setenv("ZFLIST_PROGRESS", "1", true)

	zflist_run("./" + zflist_bin, ["open", zdbfs_file], false)
	zflist_run("./" + zflist_bin, ["metadata", "backend", "--host", "hub.grid.tf", "--port", "9900"], false)

	files := [
		"/bin/zdb",
		"/bin/zdbctl",
		"/bin/zdbfs",
		"/bin/zstor-v2",
		"/bin/fusermount3"
		"/var/lib/zdb-hook.sh",
	]

	for file in files {
		print("[+] extracting: " + file + " ")

		target := os.join_path(rootdir, file)

		zflist_run("./" + zflist_bin, ["get", file, target], true)
		os.chmod(target, 0o775) or { eprintln(err) }
	}

	zflist_run("./" + zflist_bin, ["close"], false)

	// symlink zstor binary name
	os.chdir(os.join_path(rootdir, "bin")) or { eprintln(err) }
	os.symlink("zstor-v2", "zstor") or { return }

	os.chdir(rootdir) or { eprintln(err) }
}

fn zstor_precheck(rootdir string) bool {
	println("[+] checking for a running zstor daemon")

	if os.exists(rootdir + "/var/tmp/zstor.sock") {
		println("[+] zstor is already running")
		return true
	}

	return false
}

fn zstor_init(rootdir string) bool {
	println("[+] starting zstor daemon")

	mut zdb := os.new_process(rootdir + "/bin/zstor-v2")
	zargs := [
		"-c", rootdir + "/etc/zstor-default.toml",
		"monitor"
	]

	zdb.set_args(zargs)
	zdb.set_redirect_stdio()
	zdb.run()

	return true
}

fn zdb_precheck(rootdir string, port int) bool {
	println("[+] checking for a local 0-db")

	mut conn := net.dial_tcp("127.0.0.1:$port") or {
		// this is ignored, bug filled
		return false
	}

	conn.write_string("#1") or {
		if err.code() == net.err_new_socket_failed.code() {
			// connection refused
			return false
		}

		// we return false anyway on error
		return false
	}

	println("[+] local 0-db already running")
	return true
}

fn zdb_local_init(rootdir string, port int) bool {
	println("[+] starting 0-db local backend")

	mut zdb := os.new_process(rootdir + "/bin/zdb")
	zargs := [
		"--index", rootdir + "/var/tmp/zdb/index-backend",
		"--data", rootdir + "/var/tmp/zdb/data-backend",
		"--port", "$port",
		"--background"
	]

	zdb.set_args(zargs)
	zdb.set_redirect_stdio()
	zdb.run()
	zdb.wait()

	if zdb.code != 0 {
		eprintln("[-] could not start 0-db backend")
		return false
	}

	namespaces := ["zstor-meta-1", "zstor-meta-2", "zstor-meta-3", "zstor-meta-4", "zstor-backend-1", "zstor-backend-2"]

	// create all namespaces
	for namespace in namespaces {
		mut zdbctl := os.new_process(rootdir + "/bin/zdbctl")
		zenvs := {"ZDBCTL_PORT": "$port"}

		zctlargs := ["NSNEW", namespace]

		zdbctl.set_environment(zenvs)
		zdbctl.set_redirect_stdio()
		zdbctl.set_args(zctlargs)
		zdbctl.run()
		zdbctl.wait()
	}

	// set correct mode for namespaces
	for namespace in namespaces {
		mut zdbctl := os.new_process(rootdir + "/bin/zdbctl")
		zenvs := {"ZDBCTL_PORT": "$port"}

		mut mode := "seq"

		if namespace[0..10] == "zstor-meta" {
			mode = "user"
		}

		zctlargs := ["NSSET", namespace, "mode", mode]

		zdbctl.set_environment(zenvs)
		zdbctl.set_redirect_stdio()
		zdbctl.set_args(zctlargs)
		zdbctl.run()
		zdbctl.wait()
	}

	return true
}

fn zdb_init(rootdir string, port int) bool {
	println("[+] starting 0-db local cache")

	mut zdb := os.new_process(rootdir + "/bin/zdb")
	zargs := [
		"--index", rootdir + "/var/tmp/zdb/index",
		"--data", rootdir + "/var/tmp/zdb/data",
		"--hook", rootdir + "/var/lib/zdb-hook.sh",
		"--port", "$port",
		"--datasize", "33554432",  // 32 MB
		"--rotate", "900",  // 30 min
		"--mode", "seq",
		"--background"
	]

	// set zdb-hook prefix environment
	zenvs := {"ZDBFS_PREFIX": rootdir}

	zdb.set_args(zargs)
	zdb.set_environment(zenvs)
	zdb.set_redirect_stdio()
	zdb.run()
	zdb.wait()

	if zdb.code != 0 {
		eprintln("[-] could not start 0-db, some precheck failed")
		return false
	}

	return true
}

fn filesystem(rootdir string) bool {
	println("[+] starting planetary filesystem")

	mut ps := os.new_process(rootdir + "/bin/zdbfs")
	args := [
		"-o", "autons",
		"-o", "background",
		"-o", "allow_other",
		rootdir + "/mnt/zdbfs"
	]

	mut envs := map[string]string{}

	check := os.execute("fusermount3 --version")
	if check.exit_code != 0 {
		envs["PATH"] = rootdir + "/bin"
		os.Result{}
	}

	ps.set_args(args)
	ps.set_environment(envs)
	ps.run()
	ps.wait()

	return true
}

fn config_set(config string, name string, value string) string {
	query := r'(' + name + r' = )("[\w_/\-.]+")'

	mut re := regex.regex_opt(query) or { panic(err) }
	res := re.replace(config, r'\0"' + value + r'"')

	return res
}

fn config_update(path string, rootdir string) ! string {
	// read original config file
	mut data := os.read_file(path)!

	// update settings
	data = config_set(data, "root", rootdir)
	data = config_set(data, "socket", rootdir + "/var/tmp/zstor.sock")
	data = config_set(data, "zdb_data_dir_path", rootdir + "/var/tmp/zdb/data")
	data = config_set(data, "zdbfs_mountpoint", rootdir + "/mnt/zdbfs")

	// overwrite config file
	os.write_file(path, data)!

	return data
}

fn main() {
	version := "0.1.0"
	println("[+] planetary filesystem boostrap v" + version)

	mut resources := Resources{
		zflist: "https://github.com/threefoldtech/0-flist/releases/download/v2.0.1/zflist-2.0.1-amd64-linux-gnu",
		// zdbfs: "https://hub.grid.tf/maxux42.3bot/zdbfs-0.1.4-linux-gnu.flist",
		zdbfs: "https://hub.grid.tf/maxux42.3bot/zdbfs-image.flist",
		zstor: "https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/config/zstor-sample-local.toml",
	}

	home := os.getenv("HOME")
	rootdir := os.join_path(home, ".threefold")
	println("[+] prefix: " + rootdir)

	if os.args.len > 1 {
		resources.zstor = os.args[1]
		println("[+] configuration file: " + resources.zstor)
	}

	if mount_check(rootdir) == false {
		println("[-] planetary filesystem already mounted")
		exit(1)
	}

	prefix(rootdir)
	os.chdir(rootdir + "/var/cache") or { eprintln(err) }

	download(resources)
	extract(rootdir, resources)

	println("[+] updating: local zstor configuration file")
	config_update(rootdir + "/etc/zstor-default.toml", rootdir)!

	islocal := (resources.zstor == "https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/config/zstor-sample-local.toml")

	if islocal {
		if zdb_precheck(rootdir, 9901) == false {
			if zdb_local_init(rootdir, 9901) == false {
				return
			}
		}
	}

	if zstor_precheck(rootdir) == false {
		if zstor_init(rootdir) == false {
			return
		}
	}

	if zdb_precheck(rootdir, 9900) == false {
		if zdb_init(rootdir, 9900) == false {
			return
		}
	}

	filesystem(rootdir)
}
