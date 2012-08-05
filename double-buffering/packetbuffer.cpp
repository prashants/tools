/*
 * packetbuffer - Double buffering implementation
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

#include "packetbuffer.h"
#include <QFile>
#include <QDir>
#include <qDebug>

PacketBuffer::PacketBuffer()
{
    // Empty constructor
}

/*
 * This function is a public function that accepts the data filename as the first
 * parameter. It is reponsible for zeroing out the ping and pong buffers and resetting
 * all the counter values to their initial state.
 */
int PacketBuffer::initBuffer()
{
    int counter = 0;

    // initialize both the PING and PONG buffers and set the active buffer to PING
    for (counter = 0; counter < PB_MAX_BUFFER_SIZE; counter++) {
        pingBuffer[counter] = 0;
    }
    for (counter = 0; counter < PB_MAX_BUFFER_SIZE; counter++) {
        pongBuffer[counter] = 0;
    }
    bufferIndex = 0;
    activeBuffer = PING;
    pingCounter = 0;
    pongCounter = 0;

    curFileCount = 0;
    curFileName = "";
    curFolderName = "";
    packetPerFileCount = 0;

    // Creating and initializing shared moemory for storing latest packets to be shown in GUI
    sharedMemory.setKey("TESTAPP");
    if (!sharedMemory.create(PB_SHARED_MEMORY_SIZE + sizeof(sharedMemoryCounter))) {
        qDebug() << "Failed to create shared memory segment";
        return E_SMEM_FAIL_CREATE;
    }
    sharedMemoryTo = (char *)sharedMemory.data();   // Saving pointer to the shared memory data segment
    sharedMemoryTail = (unsigned long *)(sharedMemoryTo + PB_SHARED_MEMORY_SIZE);   // Saving location of the tail counter
    sharedMemoryCounter = 0;

    return SUCCESS;
}

/*
 * This function will call switchBuffer() to switch to a empty buffer and
 * force flushing the pending data to disk. It then closes the current file and
 * the shared memory segment
 */
void PacketBuffer::closeBuffer()
{
    sharedMemory.detach();
}

/*
 * This function will call switchBuffer() to switch to a empty buffer and
 * force flushing the pending data to disk. It then closes the current file.
 */
int PacketBuffer::closeDataFile()
{
    int ret;

    // switch buffers to force flush the data to disk
    ret = switchBuffer();
    if (ret != 0) {
        return ret;
    }

    // force the operating system to flush the data to disk
    ret = dataFile.flush();
    if (!ret) {
        return E_DISK_FLUSH;
    }

    dataFile.close();
    return SUCCESS;
}

// Resetting all internal counters including PING-PONG buffer, file and shared memory counter
void PacketBuffer::resetCounters()
{
    // Resetting all counter values
    bufferIndex = 0;
    activeBuffer = PING;
    pingCounter = 0;
    pongCounter = 0;

    curFileCount = 0;
    packetPerFileCount = 0;

    sharedMemoryCounter = 0;
    *sharedMemoryTail = sharedMemoryCounter;
}

/*
 * This function sets the current folder name and also the initial filename to be used
 * later on. This function also resets all counter values in use and closes any data files
 * that were previously opened. This makes sure that the application is starting clean
 * after calling this function.
 */
int PacketBuffer::setFolderName(QString outputFolderName, QString initialFileName)
{
    // Closing any previous file if open and resetting the current file and folder name
    dataFile.close();
    curFileName = "";
    curFolderName = "";

    resetCounters();    // Reset all internal counter values including PING-PONG buffer, file and shared memory counter

    // ************ FRESH START OF THE APPLICATION FROM HEREON ***************** //

    // check if folder already exists, if not create a new folder
    if (QDir(outputFolderName).exists()) {
        qDebug() << "Folder " << outputFolderName << " already exists";
    } else if (!QDir().mkpath(outputFolderName)) {                  // QDir()::mkpath() will create all parent folders if necessary
        qDebug() << "Failed to create folder " << outputFolderName;
        curFolderName = "";
        return E_FOLDER_ERROR;
    }
    curFolderName = outputFolderName;

    curFileCount = 0;

    dataFile.setFileName(curFolderName + "/" + initialFileName + ".csv");
    if (!dataFile.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qDebug() << dataFile.errorString();
        return E_FILE_ERROR;
    }

    dataStream.setDevice(&dataFile);
    if (dataStream.status() != QTextStream::Ok) {
        return E_STREAM_ERROR;
    }

    curFileName = initialFileName;

    // Update the data from the channelSetttings file
    translator.updateSettings();

    // Set the maximum buffer size depending on the sampling rate. If the sampling rate is less then 5
    // then switch after every addpacket.
    if (translator.samplingRate > 5) {
        maxBuffer = ((translator.samplingRate / 5) * PB_PACKET_SIZE);
    } else {
        maxBuffer = PB_PACKET_SIZE;
    }
    maxUseBuffer = maxBuffer - PB_PACKET_SIZE;

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
    if (bufferIndex > maxUseBuffer)
        return E_BUFFER_FULL;

    // add data packet to the current active buffer and increament the bufferIndex
    if (activeBuffer == PING) {
        // add packet to ping buffer
        for (counter = 0; counter < PB_PACKET_SIZE; counter++) {
            pingBuffer[bufferIndex] = *(data + counter);
            bufferIndex++;
        }
    } else {
        // add packet to pong buffer
        for (counter = 0; counter < PB_PACKET_SIZE; counter++) {
            pongBuffer[bufferIndex] = *(data + counter);
            bufferIndex++;
        }
    }

    // check if current buffer is full then switch buffers
    if (bufferIndex >= maxBuffer) {
        ret = switchBuffer();
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
    if (activeBuffer == PING) {
        // check if the pong buffer is empty before switching
        if (pongCounter != 0)
            return E_SWITCH_TO_NON_EMPTY;

        activeBuffer = PONG;
        // if bufferIndex is already zero then directly set the pingCount to zero
        if (bufferIndex == 0)
            pingCounter = 0;
        else
            pingCounter = bufferIndex - 1;
    } else {
        // check if the ping buffer is empty before switching
        if (pingCounter != 0)
            return E_SWITCH_TO_NON_EMPTY;

        activeBuffer = PING;
        // if bufferIndex is already zero then directly set the pongCount to zero
        if (bufferIndex == 0)
            pongCounter = 0;
        else
           pongCounter = bufferIndex - 1;
    }

    // reset the new buffer index to zero
    bufferIndex = 0;

    // flush the other buffer to disk to empty it
    ret = flushData();
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
 * it will call the changeFileName() function. Also copy the non-active buffer to
 * a shared memory segment shared with the GUI application and updated the counter
 * located at the end of the shared memory
 */
int PacketBuffer::flushData(void) {
    unsigned int counter = 0;
    int ret;

    // If the number of packets per file reaches the max then
    // change the filename
    if (packetPerFileCount >= PB_MAX_PACKET_PER_FILE) {
        ret = changeFileName();
        if (ret != 0) {
            return ret;
        }
    }

    // Flush the alternate buffer to disk and set its pingCount/pongCount to zero after flushing all the data
    if (activeBuffer == PING) {
        // check if there is any data to flush
        if (pongCounter == 0)
            return SUCCESS;

        // flush entire buffer data to disk
        for (counter = 0; counter < pongCounter; counter += PB_PACKET_SIZE) {
            dataStream << translator.convertToHuman(&pongBuffer[counter]);
            packetPerFileCount++;
        }

        // copying data to shared memory for display on the GUI
        memcpy((sharedMemoryTo + sharedMemoryCounter), &pongBuffer, pongCounter + 1);   // since zero index[n]. size is n + 1
        sharedMemoryCounter += pongCounter + 1;
        // If the shared memory counter reaches the maximum then reset it
        if (sharedMemoryCounter >= PB_SHARED_MEMORY_SIZE) {
            sharedMemoryCounter = 0;
        }
        *sharedMemoryTail = sharedMemoryCounter;

        pongCounter = 0;
    } else {
        // check if there is any data to flush
        if (pingCounter == 0)
            return SUCCESS;

        // flush entire buffer data to disk
        for (counter = 0; counter < pingCounter; counter += PB_PACKET_SIZE) {
            dataStream << translator.convertToHuman(&pingBuffer[counter]);
            packetPerFileCount++;
        }

        // copying data to shared memory for display on the GUI
        memcpy((sharedMemoryTo + sharedMemoryCounter), &pingBuffer, pingCounter + 1);   // since zero index[n]. size is n + 1
        sharedMemoryCounter += pingCounter + 1;
        // If the shared memory counter reaches the maximum then reset it
        if (sharedMemoryCounter >= PB_SHARED_MEMORY_SIZE) {
            sharedMemoryCounter = 0;
        }
        *sharedMemoryTail = sharedMemoryCounter;

        pingCounter = 0;
    }

    // force the operating system to flush the data to disk
    ret = dataFile.flush();
    if (!ret) {
        qDebug() << dataFile.errorString();
        return dataFile.error();
    }

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
    ret = dataFile.flush();
    if (!ret) {
        qDebug() << dataFile.errorString();
        return dataFile.error();
    }
    dataFile.close();

    // resetting the packet per file counter to zero to start couunting again
    packetPerFileCount = 0;
    // increament the filename counter which is appended to the filename
    curFileCount++;

    // open a new file by appending the curFileCount value
    dataFile.setFileName(curFolderName + "/" + curFileName + "_" + QString::number(curFileCount) + ".csv");
    if (!dataFile.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qDebug() << dataFile.errorString();
        return E_FILE_ERROR;
    }

    dataStream.setDevice(&dataFile);
    if (dataStream.status() != QTextStream::Ok) {
        return E_STREAM_ERROR;
    }

    return SUCCESS;
}
