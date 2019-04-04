# DiskDuplicator
DiskDuplicator takes a dd-created disk image file and writes it to USB drives inserted into the host machine. Once the image has been written DiskDuplicator reads back the image, calculates an MD5 checksum and verifies the resulting checksum against the original file. It only works on Linux.

This application was written to make duplication to USB flash drives on Linux both fast and reliable, specifically for distributing video files.

It was originally written for my own quick-and-dirty use, but I figured it might be useful.

### Process
DiskDuplicator is kind of dangerous; it requires permissions that allow it to write directly to disks, which means root permissions are necessary. I've written several thousand drives using this thing and it hasn't blitzed my host machine's drives, so I feel it's safe haha.

Start the application with root priviliges like so : sudo diskduplicator -image-path="image.img"
The application will disable Gnome's automount feature (and re-enable it when it exits).
The application will read the source image and MD5-sum it.
It'll then report that it is ready. This can be monitored by tailing the log file : tail -f errors.log

At this point, ANY newly inserted USB drive connected to the host machine will be overwritten with your source image. So start by inserting USB flash drives, one after the other, generally with a delay. DiskDuplicator will launch a goroutine for each inserted drive, write the image and then read it back*.

A good image write will be logged as such in errors.log :
```2019/04/04 13:16:04 GOOD :  /mnt/fioa/FDC19/image.img /dev/disk/by-id/usb-Generic_Flash_Disk_013D7291-0:0```

I find that it's useful to monitor the kernel message output while inserting drives : dmesg -w
as I've personally found *many* flash drives to fail on insert or shortly after.

![Alt text](disk-detected.jpg?raw=true "disk detected")
(example of a disk detected by the kernel - this one went on to fail)

![Alt text](existing-disks.jpg?raw=true "existing disks detected")
errors.log during application startup

![Alt text](in-runtime.jpg?raw=true "console during image write")
The Application's output during the read phase.

### Why
I couldn't find anything similar on Linux when I first needed this app.

Branded flash drives, and I'm sorry to say it, but particularly Chinese branded flash drives, can be extremely nasty. Manufacturers use all kinds of tricks to it look like their flash drives work, when they don't. MD5 summing the resulting storage is a highly effective way of picking up on faults, including faults hidden by FAT bad-blocks and the like.

In my experience so far, drive failures tend to be broken down as such :
- immediate failure on insert (the kernel usually says "media not found" or "media not inserted")
- silent data corruption (drive reports a successful write but the data gets corrupted)
- silent data discard (similar to above, the data gets silently zero'ed)
- drive silently ends (reports write successful but the end of the file is zero'ed)
- drive fails during write
- kernel complains about the drive being reset, continuously

Of particular irritation is that ignorant middle-men/local suppliers often don't understand these failures and think USB flash drives *cannot fail*.

Anyway, this application detects all of that.

### Building
go get -v github.com/aquarat/diskduplicator

