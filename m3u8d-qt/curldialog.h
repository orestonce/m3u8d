#ifndef CURLDIALOG_H
#define CURLDIALOG_H

#include <QDialog>
#include "m3u8d.h"

namespace Ui {
class CurlDialog;
}

class CurlDialog : public QDialog
{
    Q_OBJECT

public:
    explicit CurlDialog(QWidget *parent = 0);
    ~CurlDialog();

    void SetText(QString v);
public:
    ParseCurl_Resp Resp;
private slots:
    void on_buttonBox_accepted();

private:
    Ui::CurlDialog *ui;
};

#endif // CURLDIALOG_H
