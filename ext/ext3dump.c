/*
 * ext3dump - Utility to dump raw file data from ext2/3 partition
 *
 * Written in 2012 by Prashant P Shah <pshah.mumbai@gmail.com>
 *
 * To the extent possible under law, the author(s) have dedicated
 * all copyright and related and neighboring rights to this software
 * to the public domain worldwide. This software is distributed
 * without any warranty.
 *
 * You should have received a copy of the CC0 Public Domain Dedication
 * along with this software.
 * If not, see <http://creativecommons.org/publicdomain/zero/1.0/>.
 */

#include <stdio.h>
#include <fcntl.h>
#include <stdint.h>
#include <math.h>
#include <sys/types.h>
#include <sys/stat.h>

// Standard ext3 constants
#define SUPER_BLOCK_SIZE	1024
#define MAX_BLOCK_SIZE		8192
#define MAX_INODE_SIZE		1024
#define GDT_ENTRY_SIZE		32

// File system specific variables
int INODE_SIZE;			// Inode size
int BLOCK_SIZE;			// Block size
int INODE_PER_GROUP;		// Number of inodes per block group
int BLOCKS_PER_GROUP;		// Number of blocks per block group
int TOTAL_BLOCKS;		// Total number of blocks in file system
int GDT_SIZE;			// Gblobal Descriptor Table size
int RESERVE_GDT;		// Number of blocks reserved for GDT
int SUPERBLOCK_SIZE;		// Superblock size in bytes
int FIRST_BLOCK;		// Block number of the first block

int fs;				// The /dev entry for the file system

int inode_table_addr;

int init_fs(void);
int read_gdt(int);

int main(int argc, char *argv[])
{
	int fd;
	int i = 0, c = 0;
	struct stat filestat;
	int inode_number, blk_grp_number;
	int inode_grp_offset;
	int superblock_present;
	int offset;

	uint32_t data32;
	uint16_t data16;

	// setting up buffers
	unsigned char blk[MAX_BLOCK_SIZE];
	unsigned char blk_bitmap[MAX_BLOCK_SIZE];
	unsigned char inode_bitmap[MAX_BLOCK_SIZE];
	unsigned char inode[MAX_INODE_SIZE];

	if (argc < 3) {
		printf("\nUsage : fileblock [filename] [/dev/sda1]\n\n");
		return 1;
	}

	// Open data file
	fd = open(argv[1], O_RDONLY);
	if (fd < 0) {
		printf("Error opening file\n");
		return 1;
	}
	// Open file system
	fs = open(argv[2], O_RDONLY);
	if (fs < 0) {
		printf("Error opening file system\n");
		return 1;
	}
	// Initialize all the file system specific structures
	if (init_fs() != 0) {
		printf("Error reading extfs information from super block\n");
		return 1;
	}

	// get the inode numner of file
	if (fstat(fd, &filestat) < 0) {
		printf("Error reading inode number for a file\n");
		return 1;
	}

	inode_number = filestat.st_ino;
	blk_grp_number = (inode_number / INODE_PER_GROUP);
	inode_grp_offset = ((inode_number - 1) % INODE_PER_GROUP) * INODE_SIZE;

	// Super block copy present in block number 1, 3, 5, 7, 9
	superblock_present = 0;
	if (blk_grp_number == 0)
		superblock_present = 1;
	else if (blk_grp_number == 1)
		superblock_present = 1;
	else if (blk_grp_number == 3)
		superblock_present = 1;
	else if (blk_grp_number == 5)
		superblock_present = 1;
	else if (blk_grp_number == 7)
		superblock_present = 1;
	else if (blk_grp_number == 9)
		superblock_present = 1;

	printf("Inode number: %d (offset = %d)\n", inode_number, inode_grp_offset);
	printf("Block number: %d (super block = %d)\n", blk_grp_number, superblock_present);

	close(fd);

	if (read_gdt(blk_grp_number) < 0) {
		return 1;
	}

	printf("Inode table entry at : %d\n", (inode_table_addr * BLOCK_SIZE) + inode_grp_offset);
	if (lseek(fs, (inode_table_addr * BLOCK_SIZE) + inode_grp_offset, 0) < 0) {
		printf("Failed to seek to inode table entry\n");
		return 1;
	}

	// read inode table
	read(fs, inode, INODE_SIZE);

	// decoding inode data which in is LE format
	// http://wiki.osdev.org/Ext2

	// user id
	data16 = inode[3];
	data16 = (data16 << 8) | inode[2];
	printf("User ID : %d\n", data16);

	// file size
	data32 = inode[7];
	data32 = (data32 << 8) | inode[6];
	data32 = (data32 << 8) | inode[5];
	data32 = (data32 << 8) | inode[4];
	printf("Size : %d\n", data32);

	// address of direct block 0
	data32 = inode[43];
	data32 = (data32 << 8) | inode[42];
	data32 = (data32 << 8) | inode[41];
	data32 = (data32 << 8) | inode[40];
	printf("Direct Block 0 : %d\n", data32);

	// reading inode data
	if (lseek(fs, data32 * BLOCK_SIZE, 0) < 0) {
		printf("Failed to seek to inode data block 0\n");
		return 1;
	}
	read(fs, blk, BLOCK_SIZE);
	// Showing upto max first 1024 bytes by adding \0 in the data buffer
	blk[1023] = '\0';
	printf("Data 0 : %s\n", blk);

	close(fs);

	return 0;
}

// This function reads the super block from the filesystem
// and initialize all file system specific variables
// http://wiki.osdev.org/Ext2
int init_fs(void)
{
	uint32_t data32;
	uint16_t data16;

	// buffers
	unsigned char superblk[SUPER_BLOCK_SIZE];

	// Reading file system super block
	if (lseek(fs, 1024, 0) < 0) {
		printf("Failed to seek to superblock\n");
		return 1;
	}
	read(fs, superblk, SUPER_BLOCK_SIZE);

	// Calculating various file system parameters from super block
	data32 = superblk[7];
	data32 = (data32 << 8) | superblk[6];
	data32 = (data32 << 8) | superblk[5];
	data32 = (data32 << 8) | superblk[4];
	TOTAL_BLOCKS = data32;
	printf("Total Blocks : %d\n", TOTAL_BLOCKS);

	data32 = superblk[35];
	data32 = (data32 << 8) | superblk[34];
	data32 = (data32 << 8) | superblk[33];
	data32 = (data32 << 8) | superblk[32];
	BLOCKS_PER_GROUP = data32;
	printf("Blocks per group : %d\n", BLOCKS_PER_GROUP);

	data32 = superblk[43];
	data32 = (data32 << 8) | superblk[42];
	data32 = (data32 << 8) | superblk[41];
	data32 = (data32 << 8) | superblk[40];
	INODE_PER_GROUP = data32;
	printf("Inodes group : %d\n", INODE_PER_GROUP);

	data16 = superblk[89];
	data16 = (data16 << 8) | superblk[88];
	INODE_SIZE = data16;
	printf("Inode size : %d\n", INODE_SIZE);

	data32 = superblk[27];
	data32 = (data32 << 8) | superblk[26];
	data32 = (data32 << 8) | superblk[25];
	data32 = (data32 << 8) | superblk[24];
	BLOCK_SIZE = 1024 << data32;    // Shift 1024 by data32 bits
	printf("Blocks size : %d\n", BLOCK_SIZE);

	data16 = superblk[207];
	data16 = (data16 << 8) | superblk[206];
	RESERVE_GDT = data16;
	printf("Reserve GDT size : %d\n", RESERVE_GDT);

	SUPERBLOCK_SIZE = 1;		// Super block size is always 1024 bytes = max 1 block
	printf("Super Block : %d\n", SUPERBLOCK_SIZE);

	if (BLOCK_SIZE == 1024) {
		FIRST_BLOCK = 1;
	} else {
		FIRST_BLOCK = 0;
	}
	printf("First Block : %d\n", FIRST_BLOCK);

	// Calculating number of blocks required for GDT
	data32 = ceil((float)TOTAL_BLOCKS / BLOCKS_PER_GROUP) * GDT_ENTRY_SIZE;
	GDT_SIZE = ceil((float)data32 / BLOCK_SIZE);
	printf("GDT size : %d\n", GDT_SIZE);

	return 0;
}

int read_gdt(int block_group_number)
{
	unsigned char gdt_entry[GDT_ENTRY_SIZE];
	int i;
	uint32_t data32;

	// Reading file system super block
	// Offset = 1024 for boot block + 1024 for super block
	printf("Block group offset in GDT table : %d\n", 1024 + 1024 + (block_group_number * GDT_ENTRY_SIZE));
	if (lseek(fs, 1024 + 1024 + (block_group_number * GDT_ENTRY_SIZE), 0) < 0) {
		printf("Failed to seek to GDT entry for the block group\n");
		return 1;
	}
	read(fs, gdt_entry, GDT_ENTRY_SIZE);

	data32 = gdt_entry[3];
	data32 = (data32 << 8) | gdt_entry[2];
	data32 = (data32 << 8) | gdt_entry[1];
	data32 = (data32 << 8) | gdt_entry[0];
	printf("Block usage bitmap : %d\n", data32);

	data32 = gdt_entry[7];
	data32 = (data32 << 8) | gdt_entry[6];
	data32 = (data32 << 8) | gdt_entry[5];
	data32 = (data32 << 8) | gdt_entry[4];
	printf("Inode usage bitmap : %d\n", data32);

	data32 = gdt_entry[11];
	data32 = (data32 << 8) | gdt_entry[10];
	data32 = (data32 << 8) | gdt_entry[9];
	data32 = (data32 << 8) | gdt_entry[8];
	inode_table_addr = data32;
	printf("Inode table : %d\n", data32);

	return 0;
}
