/*
 * blockcompare - Program to compare two devices at block level
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
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <linux/fs.h>

#define BUF_SIZE	4096	/* Block size */

int main(int argc, char *argv[])
{
	int fd1 = 0, fd2 = 0;
	unsigned char *buf1 = NULL;
	unsigned char *buf2 = NULL;
	unsigned long c = 0, min = 0;
	int res = 0;

	if (argc != 3) {
		fprintf(stderr, "Invalid number of parameters.\n");
		return EXIT_FAILURE;
	}

	/* Opening block devices */
	fd1 = open(argv[1], O_RDONLY);
	if (fd1 < 0) {
		fprintf(stderr, "Failed to open first block device.\n");
		goto err;
	}
	fd2 = open(argv[2], O_RDONLY);
	if (fd2 < 0) {
		fprintf(stderr, "Failed to open second block device.\n");
		goto err;
	}

	/* Calculating size of block devices in sector size */
	ioctl(fd1, BLKGETSIZE, &c);
	fprintf(stdout, "First block device size: %lu\n", c);
	min = c;
	ioctl(fd2, BLKGETSIZE, &c);
	fprintf(stdout, "Second block device size: %lu\n", c);

	/* Partition with minimum size */
	if (c < min) {
		min = c;
	}

	/* Converting size to number of blocks */
	min = min / 8;

	/* Allocating buffers */
	buf1 = (unsigned char *)malloc(BUF_SIZE);
	if (!buf1) {
		fprintf(stderr, "Failed to allocate memory.\n");
		goto err;
	}
	memset(buf1, 0x00, BUF_SIZE);
	buf2 = (unsigned char *)malloc(BUF_SIZE);
	if (!buf2) {
		fprintf(stderr, "Failed to allocate memory.\n");
		goto err;
	}
	memset(buf2, 0x00, BUF_SIZE);

	fprintf(stdout, "Reading %lu blocks...\n", min);

	for (c = 0; c < min; c++) {
		memset(buf1, 0x00, BUF_SIZE);
		memset(buf2, 0x00, BUF_SIZE);

		/* Read one block of data from both devices */
		res = read(fd1, buf1, BUF_SIZE);
		if (res < BUF_SIZE) {
			fprintf(stdout, "End of first block device.\n");
			break;
		}
		res = read(fd2, buf2, BUF_SIZE);
		if (res < BUF_SIZE) {
			fprintf(stdout, "End of first block device.\n");
			break;
		}

		/* Compare blocks */
		res = memcmp(buf1, buf2, BUF_SIZE);
		if (res != 0) {
			fprintf(stdout, "Block differ at block number : %lu\n", c);
		}
	}

	if (buf1)
		free(buf1);
	if (buf2)
		free(buf2);
	if (fd1 > 0)
		close(fd1);
	if (fd2 > 0)
		close(fd2);

	return EXIT_SUCCESS;

err:
	if (buf1)
		free(buf1);
	if (buf2)
		free(buf2);
	if (fd1 > 0)
		close(fd1);
	if (fd2 > 0)
		close(fd2);

	return EXIT_FAILURE;
}
