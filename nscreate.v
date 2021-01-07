import vredis2

mut redis := vredis2.connect('localhost:9900')?

println(redis.send_str(["NSNEW", "zdbfs-meta"]))
println(redis.send_str(["NSNEW", "zdbfs-data"]))
println(redis.send_str(["NSNEW", "zdbfs-temp"]))
println(redis.send_str(["NSSET", "zdbfs-temp", "password", "hello"]))
println(redis.send_str(["NSSET", "zdbfs-temp", "public", "0"]))

