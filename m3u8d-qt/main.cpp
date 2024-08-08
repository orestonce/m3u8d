#include "mainwindow.h"
#include <QApplication>
#include <iostream>

int main(int argc, char *argv[])
{
    if(argc >= 2 && QString(argv[1]) == "test-startup")
    {
        //用于测试编译出来的二进制是否可以运行
        std::cerr << "The test of startup.\n";
        return 0;
    }
    QApplication a(argc, argv);
    MainWindow w;
    w.show();

    return a.exec();
}
