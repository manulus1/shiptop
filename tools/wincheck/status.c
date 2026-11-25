#define WIN32_NO_STATUS
#include <windows.h>
#undef WIN32_NO_STATUS
#include <ntstatus.h>
_Static_assert(STATUS_INFO_LENGTH_MISMATCH == (NTSTATUS)0xC0000004L, "mismatch");
_Static_assert(STATUS_SUCCESS == 0, "success");
