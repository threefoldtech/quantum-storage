#!/bin/sh

# Wait for zdbs to come up
sleep 1

# Create namespaces for backends
redis-cli -p 9901 NSNEW data1
redis-cli -p 9901 NSSET data1 password zdbpassword
redis-cli -p 9901 NSSET data1 mode seq

redis-cli -p 9901 NSNEW meta1
redis-cli -p 9901 NSSET meta1 password zdbpassword

redis-cli -p 9902 NSNEW data2
redis-cli -p 9902 NSSET data2 password zdbpassword
redis-cli -p 9902 NSSET data2 mode seq

redis-cli -p 9902 NSNEW meta2
redis-cli -p 9902 NSSET meta2 password zdbpassword

redis-cli -p 9903 NSNEW data3
redis-cli -p 9903 NSSET data3 password zdbpassword
redis-cli -p 9903 NSSET data3 mode seq

redis-cli -p 9903 NSNEW meta3
redis-cli -p 9903 NSSET meta3 password zdbpassword

redis-cli -p 9904 NSNEW data4
redis-cli -p 9904 NSSET data4 password zdbpassword
redis-cli -p 9904 NSSET data4 mode seq

redis-cli -p 9904 NSNEW meta4
redis-cli -p 9904 NSSET meta4 password zdbpassword
