#-------------------------------------------------
#
# Project created by QtCreator 2022-05-12T07:02:10
#
#-------------------------------------------------

QT       += core gui

greaterThan(QT_MAJOR_VERSION, 4): QT += widgets

TARGET = m3u8d-qt
TEMPLATE = app

# The following define makes your compiler emit warnings if you use
# any feature of Qt which has been marked as deprecated (the exact warnings
# depend on your compiler). Please consult the documentation of the
# deprecated API in order to know how to port your code away from it.
DEFINES += QT_DEPRECATED_WARNINGS

# You can also make your code fail to compile if you use deprecated APIs.
# In order to do so, uncomment the following line.
# You can also select to disable deprecated APIs only up to a certain version of Qt.
#DEFINES += QT_DISABLE_DEPRECATED_BEFORE=0x060000    # disables all the APIs deprecated before Qt 6.0.0

RC_FILE += version.rc
SOURCES += \
        main.cpp \
        mainwindow.cpp \
    m3u8d.cpp

HEADERS += \
        mainwindow.h \
    m3u8d.h

FORMS += \
        mainwindow.ui

LIBS += -L$$PWD -lm3u8d-impl
