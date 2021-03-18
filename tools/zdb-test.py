import toml
import redis

with open('default-sample.toml') as f:
    config = f.read()

data = toml.loads(config)

for group in data['groups']:
    item = group['backends'][0]
    host = item['address'][1:-6]
    port = 9900
    name = item['namespace']
    pwds = item['password']
    print(host, port)

    r = redis.Redis(host, port)
    try:
        print(r.execute_command("SELECT %s %s" % (name, pwds)))

    except Exception as e:
        print(e)
        pass
