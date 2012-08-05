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

#ifndef TRANSLATOR_H
#define TRANSLATOR_H

#include <QObject>

class Translator : public QObject
{
    Q_OBJECT
public:
    explicit Translator(QObject *parent = 0);
    unsigned int samplingRate;
    unsigned int numberOfChannels;

signals:

public slots:
    bool updateSettings();
    QString convertToHuman(unsigned char *str);

private:
    QString channelCaption[16];
    unsigned int channelEnabled[16];
    unsigned char channelParameter[16];
    float channelM[16];
    float channelC[16];
};

#endif // TRANSLATOR_H
