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

#include "packetbuffer.h"
#include <QFile>
#include <qDebug>

PacketBuffer::PacketBuffer()
{
}

/*
 * This function is a public function that accepts the data filename as the first
 * parameter. It is reponsible for zeroing out the ping and pong buffers and resetting
 * all the counter values to their initial state. It then opens the data file in the write
 * mode.
 */
int PacketBuffer::initBuffer(QString outputFileName)
{
    int counter = 0;

    // check for valid input data
    if (outputFileName.size() < 1)
        return E_INVALID_PARAM;

    // initialize both the PING and PONG buffers and set the active buffer to PING
    for (counter = 0; counter < PB_BUFFER_SIZE; counter++) {
        this->pingBuffer[counter] = 0;
    }
    for (counter = 0; counter < PB_BUFFER_SIZE; counter++) {
        this->pongBuffer[counter] = 0;
    }
    this->bufferIndex = 0;
    this->activeBuffer = PING;
    this->pingCount = 0;
    this->pongCount = 0;

    this->packetPerFileCount = 0;
    this->curFileCount = 0;
    this->curFileName = outputFileName;

    this->dataFile.setFileName(this->curFileName + ".csv");
    if (!this->dataFile.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qDebug() << this->dataFile.errorString();
        return E_FILE_ERROR;
    }

    this->dataStream.setDevice(&this->dataFile);
    if (this->dataStream.status() != QTextStream::Ok) {
        return E_STREAM_ERROR;
    }

    // Creating and initializing shared moemory for storing latest packets to be shown in GUI
    this->sharedMemory.setKey("ATOMBERG");
    this->sharedMemorySize = sizeof(this->pingBuffer);
    if (!this->sharedMemory.create(this->sharedMemorySize)) {
        return E_SMEM_FAIL_CREATE;
    }
    this->sharedMemoryTo = (char *)this->sharedMemory.data();   // Saving pointer to the shared memory data segment

    return SUCCESS;
}

/*
 * This function will call switchBuffer() to switch to a empty buffer and
 * force flushing the pending data to disk. It then closes the current file.
 */
int PacketBuffer::closeBuffer()
{
    int ret;

    // switch buffers to force flush the data to disk
    ret = this->switchBuffer();
    if (ret != 0) {
        return ret;
    }

    // force the operating system to flush the data to disk
    ret = this->dataFile.flush();
    if (!ret) {
        return E_DISK_FLUSH;
    }

    this->dataFile.close();
    return SUCCESS;
}

/*
 * This function adds the data packet recieved to the ping or pong buffer. It first checks whether
 * enough free space is available in the currently active buffer for the packet. After adding the packet
 * it checks if the buffer is full on which it will call switchBuffer() to switch to the other buffer.
 * The switch function will handle flushing the data to the disk and emptying the previous buffer.
 */
int PacketBuffer::addPacket(unsigned char *data) {
    unsigned int counter = 0;
    int ret;

    // check if enough space is available in the current buffer
    if (this->bufferIndex > PB_MAX_USED_BUFFER)
        return E_BUFFER_FULL;

    // add data packet to the current active buffer and increament the bufferIndex
    if (this->activeBuffer == PING) {
        // add packet to ping buffer
        for (counter = 0; counter < PB_PACKET_SIZE; counter++) {
            this->pingBuffer[this->bufferIndex] = *(data + counter);
            this->bufferIndex++;
        }
    } else {
        // add packet to pong buffer
        for (counter = 0; counter < PB_PACKET_SIZE; counter++) {
            this->pongBuffer[this->bufferIndex] = *(data + counter);
            this->bufferIndex++;
        }
    }

    // check if current buffer is full then switch buffers
    if (this->bufferIndex == PB_BUFFER_SIZE) {
        ret = this->switchBuffer();
        if (ret != 0) {
            return ret;
        }
    }

    return SUCCESS;
}

/*
 * This function will switch the currently active buffer. It checks whether the buffer it is
 * switching to has been flushed to disk before switching to it else the data will be lost.
 * After switching the buffer it calls the flushData() method to flush the previous buffers
 * data to disk.
 */
int PacketBuffer::switchBuffer(void) {
    int ret;

    // Switch the buffer, save the current bufferIndex to the corresponding pingCount/pongCount
    if (this->activeBuffer == PING) {
        // check if the pong buffer is empty before switching
        if (this->pongCount != 0)
            return E_SWITCH_TO_NON_EMPTY;

        this->activeBuffer = PONG;
        // if bufferIndex is already zero then directly set the pingCount to zero
        if (this->bufferIndex == 0)
            this->pingCount = 0;
        else
            this->pingCount = this->bufferIndex - 1;
    } else {
        // check if the ping buffer is empty before switching
        if (this->pingCount != 0)
            return E_SWITCH_TO_NON_EMPTY;

        this->activeBuffer = PING;
        // if bufferIndex is already zero then directly set the pongCount to zero
        if (this->bufferIndex == 0)
            this->pongCount = 0;
        else
           this->pongCount = this->bufferIndex - 1;
    }

    // reset the new buffer index to zero
    this->bufferIndex = 0;

    // flush the other buffer to disk to empty it
    ret = this->flushData();
    if (ret != 0) {
        return ret;
    }

    return SUCCESS;
}

/*
 * This function will flush the data from the non-active buffer to disk and reset
 * the buffer counter value to zero. At the end of this function a flush is called
 * on the current active file to force operating system to flush data to disk.
 * This function also will check if the max packet per file count is reached, on which
 * it will call the changeFileName() function.
 */
int PacketBuffer::flushData(void) {
    unsigned int counter = 0;
    int ret;

    // If the number of packets per file reaches the max then
    // change the filename
    if (this->packetPerFileCount >= PB_MAX_PACKET_PER_FILE) {
        ret = this->changeFileName();
        if (ret != 0) {
            return ret;
        }
    }

    // Flush the alternate buffer to disk and set its pingCount/pongCount to zero after flushing all the data
    if (this->activeBuffer == PING) {
        // check if there is any data to flush
        if (this->pongCount == 0)
            return SUCCESS;

        // flush entire buffer data to disk
        for (counter = 0; counter <= this->pongCount; counter++) {
            this->dataStream << this->pongBuffer[counter];
            if (((counter + 1) % PB_PACKET_SIZE) == 0) {
                this->dataStream << "\n";
                this->packetPerFileCount++;
            } else {
                this->dataStream << ",";
            }
        }
        this->pongCount = 0;
    } else {
        // check if there is any data to flush
        if (this->pingCount == 0)
            return SUCCESS;

        // flush entire buffer data to disk
        for (counter = 0; counter <= this->pingCount; counter++) {
            this->dataStream << this->pingBuffer[counter];
            if (((counter + 1) % PB_PACKET_SIZE) == 0) {
                this->dataStream << "\n";
                this->packetPerFileCount++;
            } else {
                this->dataStream << ",";
            }
        }
        this->pingCount = 0;
    }

    // force the operating system to flush the data to disk
    ret = this->dataFile.flush();
    if (!ret) {
        qDebug() << this->dataFile.errorString();
        return this->dataFile.error();
    }

    // copying data to shared memory for display on the GUI
    this->sharedMemory.lock();
    if (this->activeBuffer == PING) {
        memcpy(this->sharedMemoryTo, &this->pongBuffer, this->sharedMemorySize);
    } else {
        memcpy(this->sharedMemoryTo, &this->pingBuffer, this->sharedMemorySize);
    }
    this->sharedMemory.unlock();

    return SUCCESS;
}

/*
 * This function changed the current data file name when the maximum packet that can be
 * saved in a file limit is reached. This limit is checked at the end of every flushData()
 * function. On calling this function the current data file name is appened with a
 * underscore followed by counter value. eg : datafile_1.csv, datafile_2.csv, etc.
 */
int PacketBuffer::changeFileName()
{
    int ret;

    // force the operating system to flush the data to disk and close the current file
    ret = this->dataFile.flush();
    if (!ret) {
        qDebug() << this->dataFile.errorString();
        return this->dataFile.error();
    }
    this->dataFile.close();

    // resetting the packet per file counter to zero to start couunting again
    this->packetPerFileCount = 0;
    this->curFileCount++;

    // open a new file by appending the curFileCount value
    this->dataFile.setFileName(this->curFileName + "_" + QString::number(this->curFileCount) + ".csv");
    if (!this->dataFile.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qDebug() << this->dataFile.errorString();
        return E_FILE_ERROR;
    }

    this->dataStream.setDevice(&this->dataFile);
    if (this->dataStream.status() != QTextStream::Ok) {
        return E_STREAM_ERROR;
    }

    return SUCCESS;
}
