ts3snap - Create and restore TeamSpeak 3 server snapshots

Commands:

    create <filepath>
        Take a snapshot and write it to <filepath>.
        Snapshots can be encrypted with a password provided via --secret flag.

    deploy <filepath>
        Read a snapshot from <filepath> and restore it to the server.
        This is a destructive action overwriting the servers' configuration!
        Options:
            --keepfiles
                Preserve existing channel files.

    help
        Print this help message and exit.

Global options:

    --host=<address>
        Server query address
        Default value: localhost

    --port=<port>
        Server query port
        Default value: 10022

    --proto=(raw|ssh|http|https)
        Server query protocol

    --user=<name>
        Server query user
        Used in conjunction with: --pass
        Required for protocols: raw and ssh
        Default value: serveradmin

    --pass=<secret>
        Server query password
        Used in conjunction with: --user
        Required for protocols: raw and ssh

    --key=<apikey>
        Server query key
        Required for protocols: http and https

    --secret=<secret>
        Snapshot file password

    --srvid=<id>
        Virtual server id
        Default value: 1

    --url-path=<path>
        Additional url path elements

    --ssl-insecure
        Skip cert verification for https connections.

Authentication:

    For authentication with the raw and ssh protocols,
    username (--user) and password (--pass) flags are required.

    For authentication with the http and https protocols,
    the api-key flag (--key) is required.

Security:

    With protocols raw and http, the query connection is not encrypted!
    Do not use unencrypted protocols to connect across the internet!
    Instead, use ssh or configure https web query for the server.

    Please note that the TeamSpeak 3 Server does NOT check for necessary
    permissions while deploying a snapshot so the command could be abused
    to gain additional privileges.

Usage examples:

    ts3snap \
        --pass="${pass}" \
        create snapshot.json

    ts3snap \
        --host='example.org' \
        --port=8443 \
        --proto=https \
        --key="${key}" \
        --srvid=42 \
        deploy snapshot.json

    cat /tmp/snapshot.json | jq -r '.data' | base64 -d | unzstd
