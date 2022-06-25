#include "curldialog.h"
#include "ui_curldialog.h"

CurlDialog::CurlDialog(QWidget *parent) :
    QDialog(parent),
    ui(new Ui::CurlDialog)
{
    ui->setupUi(this);
}

CurlDialog::~CurlDialog()
{
    delete ui;
}

void CurlDialog::SetText(QString v)
{
    if (v.isEmpty()) {
        return;
    }
    ui->plainTextEdit->setPlainText(v);
}

void CurlDialog::on_buttonBox_accepted()
{
    ParseCurl_Resp resp= ParseCurlStr(ui->plainTextEdit->toPlainText().toStdString());
    if (resp.ErrMsg.empty() == false) {
        Toast::Instance()->SetError(resp.ErrMsg.c_str());
        this->reject();
        return;
    }
    this->Resp = resp;
}
