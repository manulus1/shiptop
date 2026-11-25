/* Compile-time proof that the offsets/sizes/constants hard-coded in sys_windows.go and
   term_windows.go match the real Windows x64 headers. Compiles only if every assert holds. */
#include <windows.h>
#include <winternl.h>
#include <psapi.h>
#include <stddef.h>

#define A(x) _Static_assert(x, #x)

/* NtQuerySystemInformation classes */
A(SystemProcessInformation == 5);
A(SystemProcessorPerformanceInformation == 8);

/* SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION: stride 48, idle/kernel/user at 0/8/16 */
A(sizeof(SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION) == 48);
A(offsetof(SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION, IdleTime) == 0);
A(offsetof(SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION, KernelTime) == 8);
A(offsetof(SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION, UserTime) == 16);

/* SYSTEM_PROCESS_INFORMATION fields read by parseProcs */
A(offsetof(SYSTEM_PROCESS_INFORMATION, NextEntryOffset) == 0);
A(offsetof(SYSTEM_PROCESS_INFORMATION, NumberOfThreads) == 4);
A(offsetof(SYSTEM_PROCESS_INFORMATION, CreateTime) == 32);
A(offsetof(SYSTEM_PROCESS_INFORMATION, UserTime) == 40);
A(offsetof(SYSTEM_PROCESS_INFORMATION, KernelTime) == 48);
A(offsetof(SYSTEM_PROCESS_INFORMATION, ImageName) == 56);
A(offsetof(UNICODE_STRING, Length) == 0);
A(sizeof(((UNICODE_STRING *)0)->Length) == 2);
A(offsetof(UNICODE_STRING, Buffer) == 8);
A(offsetof(SYSTEM_PROCESS_INFORMATION, UniqueProcessId) == 80);
A(offsetof(SYSTEM_PROCESS_INFORMATION, InheritedFromUniqueProcessId) == 88);
A(offsetof(SYSTEM_PROCESS_INFORMATION, HandleCount) == 96);
A(offsetof(SYSTEM_PROCESS_INFORMATION, VirtualMemoryCounters) + offsetof(VM_COUNTERS, WorkingSetSize) == 144);
A(sizeof(((VM_COUNTERS *)0)->WorkingSetSize) == 8);

/* MEMORYSTATUSEX */
A(sizeof(MEMORYSTATUSEX) == 64);
A(offsetof(MEMORYSTATUSEX, ullTotalPhys) == 8);
A(offsetof(MEMORYSTATUSEX, ullAvailPhys) == 16);

/* PERFORMANCE_INFORMATION (values in pages) */
A(sizeof(PERFORMANCE_INFORMATION) == 104);
A(offsetof(PERFORMANCE_INFORMATION, CommitTotal) == 8);
A(offsetof(PERFORMANCE_INFORMATION, CommitLimit) == 16);
A(offsetof(PERFORMANCE_INFORMATION, SystemCache) == 48);
A(offsetof(PERFORMANCE_INFORMATION, PageSize) == 80);
A(offsetof(PERFORMANCE_INFORMATION, HandleCount) == 88);
A(offsetof(PERFORMANCE_INFORMATION, ProcessCount) == 92);
A(offsetof(PERFORMANCE_INFORMATION, ThreadCount) == 96);

/* PROCESS_MEMORY_COUNTERS (fallback path) */
A(sizeof(PROCESS_MEMORY_COUNTERS) == 72);
A(offsetof(PROCESS_MEMORY_COUNTERS, WorkingSetSize) == 16);

/* SYSTEM_POWER_STATUS */
A(sizeof(SYSTEM_POWER_STATUS) == 12);
A(offsetof(SYSTEM_POWER_STATUS, ACLineStatus) == 0);
A(offsetof(SYSTEM_POWER_STATUS, BatteryFlag) == 1);
A(offsetof(SYSTEM_POWER_STATUS, BatteryLifePercent) == 2);

/* CONSOLE_SCREEN_BUFFER_INFO (term_windows.go csbi struct) */
A(sizeof(CONSOLE_SCREEN_BUFFER_INFO) == 22);
A(offsetof(CONSOLE_SCREEN_BUFFER_INFO, srWindow) == 10);

/* constants */
A(PROCESS_QUERY_LIMITED_INFORMATION == 0x1000);
A(PROCESS_TERMINATE == 0x0001);
A(TOKEN_QUERY == 0x0008);
A(SEM_FAILCRITICALERRORS == 0x0001);
A(SEM_NOOPENFILEERRORBOX == 0x8000);
A(DRIVE_REMOVABLE == 2);
A(DRIVE_FIXED == 3);
A(ENABLE_PROCESSED_INPUT == 0x1);
A(ENABLE_LINE_INPUT == 0x2);
A(ENABLE_ECHO_INPUT == 0x4);
A(ENABLE_QUICK_EDIT_MODE == 0x40);
A(ENABLE_EXTENDED_FLAGS == 0x80);
A(ENABLE_VIRTUAL_TERMINAL_INPUT == 0x200);
A(ENABLE_PROCESSED_OUTPUT == 0x1);
A(ENABLE_VIRTUAL_TERMINAL_PROCESSING == 0x4);
A(DISABLE_NEWLINE_AUTO_RETURN == 0x8);
A(CP_UTF8 == 65001);
A(KEY_READ == 0x20019);
A(ERROR_BUFFER_OVERFLOW == 111);
A((ULONG_PTR)HKEY_LOCAL_MACHINE == (ULONG_PTR)(LONG)0x80000002);
