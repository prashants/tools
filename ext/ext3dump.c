/*
 * Author : Prashant Shah <pshah.mumbai@gmail.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

#include <stdio.h>
#include <fcntl.h>
#include <stdint.h>
#include <math.h>
#include <sys/types.h>
#include <sys/stat.h>

#define SUPER_BLOCK_SIZE	1024
#define MAX_BLOCK_SIZE		8192
#define MAX_INODE_SIZE		1024
#define GDT_ENTRY_SIZE		32

int INODE_SIZE;
int BLOCK_SIZE;
int INODE_PER_GROUP;
int BLOCKS_PER_GROUP;
int TOTAL_BLOCKS;
int GDT_SIZE;

int fs;

int init_fs(void);

int main(int argc, char *argv[])
{
	int fd;
	int i = 0, c = 0;
	struct stat filestat;
	int inode_number, blk_grp_number;
	int inode_grp_offset;
	int superblock_present;

	uint32_t data32;
	uint16_t data16;

	// buffers
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
	// Opening file system
	fs = open(argv[2], O_RDONLY);
	if (fs < 0) {
		printf("Error opening file system\n");
		return 1;
	}
	init_fs();

	// get the inode numner of file
	if (fstat(fd, &filestat) < 0) {
		printf("Error reading inode number for a file\n");
		return 1;
	}

	inode_number = filestat.st_ino;
	blk_grp_number = (inode_number / INODE_PER_GROUP);
	inode_grp_offset = ((inode_number - 1) % INODE_PER_GROUP) * INODE_SIZE;

	// Super block copy present in block number 1, 2, and all other block numbers divisible by 3, 5, 7
	superblock_present = 0;
	if (blk_grp_number == 0)
		superblock_present = 1;
	if (blk_grp_number == 1)
		superblock_present = 1;
	if (blk_grp_number == 3)
		superblock_present = 1;
	if (blk_grp_number == 5)
		superblock_present = 1;
	if (blk_grp_number == 7)
		superblock_present = 1;
	if (blk_grp_number == 9)
		superblock_present = 1;

	printf("Inode number: %d (offset = %d)\n", inode_number, inode_grp_offset);
	printf("Block number: %d (super block = %d)\n", blk_grp_number, superblock_present);
	
	close(fd);


	// reading inode bitmap
	if (superblock_present == 1) {
		printf("Inode bitmap at : %d\n", (blk_grp_number * BLOCKS_PER_GROUP) + 259 + GDT_SIZE);
		if (lseek(fs, ((blk_grp_number * BLOCKS_PER_GROUP) + 259 + GDT_SIZE) * BLOCK_SIZE, 0) < 0) {
			printf("Failed to seek to inode bitmap\n");
			return 1;
		}
	} else {
		printf("Inode bitmap at : %d\n", (blk_grp_number * BLOCKS_PER_GROUP) + 2);
		if (lseek(fs, ((blk_grp_number * BLOCKS_PER_GROUP) + 2) * BLOCK_SIZE, 0) < 0) {
			printf("Failed to seek to inode bitmap\n");
			return 1;
		}
	}
	read(fs, inode_bitmap, BLOCK_SIZE);
	printf("Inode bitmap dump :\n");
	for (i = 0; i < BLOCK_SIZE; i++) {
		printf("%d ", inode_bitmap[i]);
	}
	printf("\n");

	// reading inode table
	if (superblock_present == 1) {
		printf("Inode table at : %d\n", (blk_grp_number * BLOCKS_PER_GROUP) + 260 + GDT_SIZE);
		printf("Inode table entry at : %d\n", (((blk_grp_number * BLOCKS_PER_GROUP) + 260 + GDT_SIZE) * BLOCK_SIZE) + inode_grp_offset);
		if (lseek(fs, (((blk_grp_number * BLOCKS_PER_GROUP) + 260 + GDT_SIZE) * BLOCK_SIZE) + inode_grp_offset, 0) < 0) {
			printf("failed to seek to inode table\n");
			return 1;
		}
	} else {
		printf("Inode table at : %d\n", (blk_grp_number * BLOCKS_PER_GROUP) + 3);
		printf("Inode table at : %d\n", (((blk_grp_number * BLOCKS_PER_GROUP) + 3) * BLOCK_SIZE) + inode_grp_offset);
		if (lseek(fs, (((blk_grp_number * BLOCKS_PER_GROUP) + 3) * BLOCK_SIZE) + inode_grp_offset, 0) < 0) {
			printf("failed to seek to inode table\n");
			return 1;
		}
	}

	read(fs, inode, INODE_SIZE);
	printf("Inode dump :\n");
	for (i = 0; i < INODE_SIZE; i++) {
		printf("%d ", inode[i]);
	}
	printf("\n");

	// decoding inode data which in is LE format
	data16 = inode[3];
	data16 = (data16 << 8) | inode[2];
	printf("User ID : %d\n", data16);

	data32 = inode[7];
	data32 = (data32 << 8) | inode[6];
	data32 = (data32 << 8) | inode[5];
	data32 = (data32 << 8) | inode[4];
	printf("Size : %d\n", data32);

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
	blk[1023] = '\0';
	printf("Data 0 : %s\n", blk);

	close(fs);

	return 0;
}

// This function will read the super block from the filesystem
// and initialize all varaibles
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

	BLOCK_SIZE = 1024;
	printf("Blocks size : %d\n", BLOCK_SIZE);

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

	// Calculating number of blocks required for GDT
	data32 = ceil((float)TOTAL_BLOCKS / BLOCKS_PER_GROUP) * GDT_ENTRY_SIZE;
	GDT_SIZE = ceil((float)data32 / BLOCK_SIZE);
	printf("GDT size : %d\n", GDT_SIZE);
	return 0;
}
