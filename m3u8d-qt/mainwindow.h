#ifndef MAINWINDOW_H
#define MAINWINDOW_H

#include <QMainWindow>
#include "m3u8d.h"

namespace Ui {
class MainWindow;
}

class MainWindow : public QMainWindow
{
    Q_OBJECT

public:
    explicit MainWindow(QWidget *parent = 0);
    ~MainWindow();

private slots:
    void on_pushButton_RunDownload_clicked();

    void on_pushButton_SaveDir_clicked();

private:
    Ui::MainWindow *ui;
    RunOnUiThread m_syncUi;
};

#endif // MAINWINDOW_H
