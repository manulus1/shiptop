/* Compile-time proof for the layouts and constants used by sys_windows_power.go
   (performance counters, SetupAPI, battery IOCTLs). Compiles only if every assert holds. */
#include <windows.h>
#include <pdh.h>
#include <pdhmsg.h>
#include <setupapi.h>
#include <batclass.h>
#include <stddef.h>

#define A(x) _Static_assert(x, #x)

/* PDH: formatted counter array items */
A(sizeof(PDH_FMT_COUNTERVALUE_ITEM_W) == 24);
A(offsetof(PDH_FMT_COUNTERVALUE_ITEM_W, szName) == 0);
A(offsetof(PDH_FMT_COUNTERVALUE_ITEM_W, FmtValue) == 8);
A(offsetof(PDH_FMT_COUNTERVALUE, CStatus) == 0);
A(offsetof(PDH_FMT_COUNTERVALUE, doubleValue) == 8);
A(PDH_FMT_DOUBLE == 0x00000200);
A(PDH_FMT_NOCAP100 == 0x00008000);
A(PDH_MORE_DATA == (LONG)0x800007D2);
A(PDH_CSTATUS_VALID_DATA == 0);
A(PDH_CSTATUS_NEW_DATA == 1);

/* SetupAPI device interface enumeration */
A(sizeof(SP_DEVICE_INTERFACE_DATA) == 32);
A(offsetof(SP_DEVICE_INTERFACE_DATA, cbSize) == 0);
A(sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA_W) == 8);
A(offsetof(SP_DEVICE_INTERFACE_DETAIL_DATA_W, DevicePath) == 4);
A(DIGCF_PRESENT == 0x2);
A(DIGCF_DEVICEINTERFACE == 0x10);

/* battery class driver IOCTLs */
A(IOCTL_BATTERY_QUERY_TAG == 0x294040);
A(IOCTL_BATTERY_QUERY_INFORMATION == 0x294044);
A(IOCTL_BATTERY_QUERY_STATUS == 0x29404C);
A(sizeof(BATTERY_WAIT_STATUS) == 20);
A(offsetof(BATTERY_WAIT_STATUS, BatteryTag) == 0);
A(offsetof(BATTERY_WAIT_STATUS, Timeout) == 4);
A(sizeof(BATTERY_STATUS) == 16);
A(offsetof(BATTERY_STATUS, PowerState) == 0);
A(offsetof(BATTERY_STATUS, Capacity) == 4);
A(offsetof(BATTERY_STATUS, Voltage) == 8);
A(offsetof(BATTERY_STATUS, Rate) == 12);
A(sizeof(BATTERY_QUERY_INFORMATION) == 12);
A(offsetof(BATTERY_QUERY_INFORMATION, BatteryTag) == 0);
A(offsetof(BATTERY_QUERY_INFORMATION, InformationLevel) == 4);
A(offsetof(BATTERY_QUERY_INFORMATION, AtRate) == 8);
A(BatteryInformation == 0);
A(BatteryTemperature == 2);
A(sizeof(BATTERY_INFORMATION) == 36);
A(offsetof(BATTERY_INFORMATION, Capabilities) == 0);
A(offsetof(BATTERY_INFORMATION, DesignedCapacity) == 12);
A(offsetof(BATTERY_INFORMATION, FullChargedCapacity) == 16);
A(offsetof(BATTERY_INFORMATION, CycleCount) == 32);
A(BATTERY_UNKNOWN_RATE == 0x80000000);
A(BATTERY_UNKNOWN_CAPACITY == 0xFFFFFFFF);
A(BATTERY_CAPACITY_RELATIVE == 0x40000000);
A(BATTERY_POWER_ON_LINE == 0x1);
A(BATTERY_DISCHARGING == 0x2);
A(BATTERY_CHARGING == 0x4);

/* SYSTEM_POWER_STATUS: remaining battery time in seconds */
A(offsetof(SYSTEM_POWER_STATUS, BatteryLifeTime) == 4);

/* CreateFileW flags */
A(GENERIC_READ == 0x80000000);
A(GENERIC_WRITE == 0x40000000);
A(FILE_SHARE_READ == 1);
A(FILE_SHARE_WRITE == 2);
A(OPEN_EXISTING == 3);
A(FILE_ATTRIBUTE_NORMAL == 0x80);
