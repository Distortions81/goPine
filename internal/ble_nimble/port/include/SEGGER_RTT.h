#ifndef GOPINE_RTT_H
#define GOPINE_RTT_H
// No debug probe/logger is required by the time-sync build.
#define SEGGER_RTT_printf(...) ((void)0)
#endif
