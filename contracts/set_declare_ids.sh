#!/bin/zsh
sed -i.bak 's/declare_id!("My11111111111111111111111111111111111111112");/declare_id!("ENmeY9iRUUzN5NUvhHTc5vA8nQuYqXcsU7h4JzKMw5aE");/g' programs/access-controller/src/lib.rs
sed -i.bak 's/declare_id!("My11111111111111111111111111111111111111113");/declare_id!("CVmCNhhYxQHRjWJuGDUio3JtNnsTfHTVWyTgq6UozQSw");/g' programs/deviation-flagging-validator/src/lib.rs
sed -i.bak 's/declare_id!("My11111111111111111111111111111111111111111");/declare_id!("Fh7uhkZLvogdxrQccDpRgMDyN3wKb5ZQGRcpvujeqAae");/g' programs/ocr2/src/lib.rs

