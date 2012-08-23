ext3dump is a tool to dump raw inode information about a file

INSTALL :
---------

1. Compile the program with gcc compiler

$gcc ext3dump.c -o ext3dump -lm

2. Run the program as root user with the filename followed
by device filename

$sudo ./ext3dump <filename> <device>

eg : $sudo ./ext3dump /etc/rc.local /dev/sda1

