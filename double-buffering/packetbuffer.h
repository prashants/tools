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

#ifndef PACKETBUFFER_H
#define PACKETBUFFER_H

#include <QObject>
#include <QFile>
#include <QTextStream>
#include <QSharedMemory>

// Packet Buffer sizes
#define PB_PACKET_SIZE         40  // Data packet size
#define PB_NUMBER_OF_PACKETS   20  // Number of packets in the active buffer to hold at a time
#define PB_BUFFER_SIZE         800 // PACKET_SIZE * NUMBER_OF_PACKETS = Total buffer size
#define PB_MAX_USED_BUFFER     760 // PB_BUFFER_SIZE - PB_PACKET_SIZE. Small optimization to check for the maximum used buffer before adding a packet

#define PB_MAX_PACKET_PER_FILE  2000 // Maximum number of packets per file after which the file name is changed

// ERROR CODES
#define SUCCESS                 0   // Everything ok
#define E_FILE_ERROR            1   // Error opening file in write mode
#define E_BUFFER_FULL           2   // Failed to switch buffer hence any futher writes to the same buffer will result in this error.
#define E_INVALID_PARAM         3   // Invalid parameter passed to public function
#define E_SWITCH_TO_NON_EMPTY   4   // Error since the program tried to switch to a buffer that is already full and not flushed to disk. If it switches then it will overwrite the data which has not been flushed to disk.
#define E_DISK_FLUSH            5   // Error in flushing data in buffers to file on Operating System side
#define E_STREAM_ERROR          6   // Error in flushing data from the stream to file
#define E_SMEM_FAIL_CREATE      7   // Failed to create shared memory segment

class PacketBuffer : public QObject
{
public:
    PacketBuffer();
    int initBuffer(QString outputFileName);
    int closeBuffer();
    int addPacket(unsigned char *data);

private:
    int switchBuffer(void);
    int flushData(void);
    int changeFileName(void);

    QFile dataFile;
    QTextStream dataStream;

    unsigned char pingBuffer[PB_BUFFER_SIZE];   // Ping buffer
    unsigned char pongBuffer[PB_BUFFER_SIZE];   // Pong buffer
    unsigned int bufferIndex;                   // Current index inside the current buffer where the next write will happen
    unsigned int pingCount;                     // Number of packets held in the ping buffer
    unsigned int pongCount;                     // Number of packets held in the pong buffer
    enum {PING, PONG} activeBuffer;             // Current active buffer

    unsigned long packetPerFileCount;           // The current number of packets written to the active file
    unsigned int curFileCount;                  // The last number appended to the filename
    QString curFileName;                        // Filename as passed by the user in the initBuffer() method

    QSharedMemory   sharedMemory;               // Shared memory for the latest packets to be shown in read time
    char *sharedMemoryTo;                       // Pointer to shared memory data segment
    int sharedMemorySize;                       // Size of shared memory VALID data part
};

#endif // PACKETBUFFER_H
