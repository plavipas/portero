#ifndef PORTERO_HANDLER_H
#define PORTERO_HANDLER_H
void porteroRunApp(void);
void porteroExitAfter(int ms);
void porteroDone(void);
char *porteroDefaultHandler(const char *scheme);
char *porteroSetDefaultHandler(const char *appPath, const char *scheme);
#endif
