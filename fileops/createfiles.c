/*
 * filecreation - Program to create many empty files for testing
 * file system performance
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
#include <time.h>
#include <stdlib.h>

#define MAX_COUNT 100000

const char alphanum[] = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";

void gen_random(char *s, const int len)
{
	int i;
	for (i = 0; i < len; i++) {
		s[i] = alphanum[rand() % (sizeof(alphanum) - 1)];
	}
	s[len] = 0;
}

int main()
{
	int i;
	FILE *f;
	char fname[15];
	time_t start, end;

	time(&start);
	for (i = 0; i < MAX_COUNT; i++) {
		gen_random(fname, 15);
		f = fopen(fname, "w");
		if (f != NULL) {
			fclose(f);
		} else {
			perror("Error creating file\n");
			exit(1);
		}
	}
	time(&end);
	printf("Time taken %f\n", difftime(end, start));
	return 0;
}
