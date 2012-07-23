/*
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

/*
 * This program is used to create many empty files for testing file system performance
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
