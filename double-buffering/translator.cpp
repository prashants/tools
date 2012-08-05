/*
 * translator - Translating raw data into human readable format
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

#include "translator.h"

#include <QFile>
#include <QDir>
#include <QDebug>

#define CHANNEL_SETTINGS_FILE_PATH    "/channelsetup.txt"

Translator::Translator(QObject *parent) :
    QObject(parent)
{
    // Default values
    samplingRate = 100;
    numberOfChannels = 16;
    for (int i = 0; i < 16; i++) {
        channelCaption[i] = "Channel" + QString::number(i);
    }
    for (int i = 0; i < 16; i++) {
        channelEnabled[i] = 1;
    }
    for (int i = 0; i < 16; i++) {
        channelParameter[i] = 'V';
    }
    for (int i = 0; i < 16; i++) {
        channelM[i] = 1;
    }
    for (int i = 0; i < 16; i++) {
        channelC[i] = 0;
    }
}

bool Translator::updateSettings()
{
    QTextStream in;
    QFile channelSetupFile(QDir::currentPath() + CHANNEL_SETTINGS_FILE_PATH);

    if (channelSetupFile.open(QIODevice::ReadOnly | QIODevice::Text))
    {
        qDebug() << "Reading channelsetup file";
        in.setDevice(&channelSetupFile);
    } else {
        qDebug() << "Error opening channelsetup file";
        return false;
    }

    // Reading the settings into local variables
    QString samplingRateStr, numberOfChannelsStr, captionStr, channelEnabledStr, parameterStr, mStr, cStr;
    bool ok;

    samplingRateStr = in.readLine();
    numberOfChannelsStr = in.readLine();
    captionStr = in.readLine();
    channelEnabledStr = in.readLine();
    parameterStr = in.readLine();
    mStr = in.readLine();
    cStr = in.readLine();

    channelSetupFile.close();

    // Default values
    samplingRate = 100;
    numberOfChannels = 16;
    for (int i = 0; i < 16; i++) {
        channelCaption[i] = "Channel" + QString::number(i);
    }
    for (int i = 0; i < 16; i++) {
        channelEnabled[i] = 1;
    }
    for (int i = 0; i < 16; i++) {
        channelParameter[i] = 'V';
    }
    for (int i = 0; i < 16; i++) {
        channelM[i] = 1;
    }
    for (int i = 0; i < 16; i++) {
        channelC[i] = 0;
    }

    // Extracting values from settings file
    samplingRate = samplingRateStr.toInt(&ok);
    numberOfChannels = numberOfChannelsStr.toInt(&ok);

    QStringList captionValues = captionStr.split(',');
    for (int i = 0; i < captionValues.size(); ++i) {
        channelCaption[i] = captionValues.at(i);
    }

    QStringList channelEnabledValues = channelEnabledStr.split(',');
    for (int i = 0; i < channelEnabledValues.size(); ++i) {
        channelEnabled[i] = channelEnabledValues.at(i).toInt(&ok);
    }

    QStringList parameterValues = parameterStr.split(',');
    for (int i = 0; i < parameterValues.size(); ++i) {
        if (parameterValues.at(i) == "V")
            channelParameter[i] = 'V';
        else
            channelParameter[i] = 'T';
    }

    QStringList mValues = mStr.split(',');
    for (int i = 0; i < mValues.size(); ++i) {
        channelM[i] = mValues.at(i).toDouble(&ok);
    }

    QStringList cValues = cStr.split(',');
    for (int i = 0; i < cValues.size(); ++i) {
        channelC[i] = cValues.at(i).toDouble(&ok);
    }

    return true;
}

// str : 40 byte input data
QString Translator::convertToHuman(unsigned char *str)
{
    unsigned int u1, u2, u3, u4, u, t, t1;
    unsigned int msec, seconds, minutes, hours;

    float f;

    // Extracting packet counter
    u1 = *(str + 32);
    u2 = *(str + 33);
    u3 = *(str + 34);
    u4 = *(str + 35);
    t = ((((u1 << 8) | u2) << 8 | u3) << 8 | u4);

    QString data, tmp;

    data = "";

    for (int count = 0; count < 16; count += numberOfChannels) {

        // Converting 4 byte packet counter to timestamp
        t1 = t;
        msec = t1 % samplingRate;
        t1 = t1 / samplingRate;
        seconds = t1 % 60;
        t1 = t1 / 60;
        minutes = t1 % 60;
        t1 = t1 / 60;
        hours = t1;

        if (samplingRate <= 10)
            data += tmp.sprintf("%d:%02d:%02d.%01d", hours, minutes, seconds, msec);
        else if (samplingRate <= 100)
            data += tmp.sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, msec);
        else
            data += tmp.sprintf("%d:%02d:%02d.%03d", hours, minutes, seconds, msec);

        data += ",";

        // Check if data is valid
        u1 = *(str + 39);
        if (u1 == 0) {                      // Missing packet dummy data received
            for (unsigned int i = 0; i < numberOfChannels; i++) {
                data += ",";
            }
            data.chop(1);   // Removing one extra comma at the end of the string
        } else {
            // Extracting channel data
            unsigned char d1, d2;
            for (unsigned int i = 0; i < numberOfChannels; i++) {
                if (channelEnabled[i] == 1) {
                    d1 = *(str + count + (i * 2));
                    d2 = *(str + count + ((i * 2) + 1));
                    if (d1 & 128) {     // check if d1 is negative or positive by AND with 1000 0000
                        if (channelParameter[i] == 'V') {  // Voltage
                            u = (d1 << 8) | d2;     // merge
                            // Taking 2's complement of a number
                            u = ~u;                 // Invert all bits by NOT
                            u = u & 0x7FFF;         // Remove all unwanted bits
                            u = u + 1;              // Add 1
                            f = (-1) * (float)u;    // make number negative

                            f = f / 8000;
                            f = (f * channelM[i]) + channelC[i];  // scaling factor
                            data += tmp.sprintf("%.5f,", f);
                        } else {                    // Temperature
                            u = (d1 << 8) | d2;     // merge
                            u = u & 0x7FFF;         // Remove all unwanted bits
                            f = (-1) * (float)u;    // make number negative

                            f = f / 10;             // If temperature already multiplied by 10 by the device, hence divide by 10
                            f = (f * channelM[i]) + channelC[i];  // scaling factor
                            data += tmp.sprintf("%.1f,", f);
                        }
                    } else {
                        // for positive values of u merge the data
                        d1 = *(str + count + (i * 2));
                        d2 = *(str + count + ((i * 2) + 1));
                        u = (d1 << 8) | d2;
                        f = (float)u;
                        // Converting int to float
                        if (channelParameter[i] == 'V') {  // Voltage
                            f = f / 8000;
                            f = (f * channelM[i]) + channelC[i];  // scaling factor
                            data += tmp.sprintf("%.5f,", f);
                        } else {                    // Temperature
                            f = f / 10;              // If temperature already multiplied by 10 by the device, hence divide by 10
                            f = (f * channelM[i]) + channelC[i];  // scaling factor
                            data += tmp.sprintf("%.1f,", f);
                        }
                    }
                } else {
                    data += ",";
                }
            } // END OF FOR - CHANNEL
            data.chop(1); // Removing one extra comma at the end of the string
        } // END OF IF - ELSE FOR VALID DATA
        data += "\n";
        t++; // Incrementing timeIndex
    } // END OF FOR - NUMBER OF CHANNELS

    return data;
}
