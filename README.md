# sshauth

A small tool for managing ssh keys.

## Build
go get github.com/otm/sshauth

## Configuration
* Create a S3 bucket. The path used will be `bucket/key/username`, and ssh public 
keys should be added there. Note, that key is optional. **Important:** only trusted
persons should have write permission to the bucket, as that will grant ssh access.
* Crate a configuration file: `/etc/sshauth/sshauth.conf` with bucket and key information
```
-bucket bucket-containig-key-conf
-key prefix
```

* Add the following configuration to your `sshd.conf` file
```
AuthorizedKeysCommand /path/to/sshauth
AuthorizedKeysCommandUser username
```
