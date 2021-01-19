# y u no internetz
internet, u ok hun?

## Permissions
`internetz` tries to send pings, which (possibly) needs elevated permissions.

`internetz` tries to make a new-ish linux _ping socket_.
It can do this if your kernel is new enough (~3.0+), and the group that's going to run `internetz` is within the range in `net.ipv4.ping_group_range`.

If not, we fall back to a raw socket, which needs capability `CAP_NET_RAW`.
You have a few options to get this permission, from insecure-but-easy, to correct-but-fiddly:
* Run as root / setuid root
* Have the cap in your process subtree's ambient or inheritable capability sets, qv
* Set the cap as permitted on the binary, and set the "auto effective" flag: `sudo setcap 'cap_net_raw+ep' ./internetz`
* Set the cap as permitted on the binary; `internetz` is capability-aware and will promote that permitted cap to an effective one for a minimal window to make its socket: `sudo setcap 'cap_net_raw+p' ./internetz`
