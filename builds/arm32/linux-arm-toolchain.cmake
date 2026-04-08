set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR arm)

set(CMAKE_C_COMPILER   /usr/bin/arm-linux-gnueabihf-gcc)
set(CMAKE_CXX_COMPILER /usr/bin/arm-linux-gnueabihf-g++)
set(CMAKE_AR           /usr/bin/arm-linux-gnueabihf-ar)
set(CMAKE_RANLIB       /usr/bin/arm-linux-gnueabihf-ranlib)
set(CMAKE_STRIP        /usr/bin/arm-linux-gnueabihf-strip)

# Cortex-A7 (STM32MP1) 针对性优化
set(ARCH_FLAGS "-mcpu=cortex-a7 -mtune=cortex-a7 -mfpu=neon-vfpv4 -mfloat-abi=hard")

set(OPT_FLAGS "-O3 -pipe -fomit-frame-pointer -funroll-loops -ftree-vectorize -DNDEBUG")

set(CMAKE_C_FLAGS   "${ARCH_FLAGS} ${OPT_FLAGS}" CACHE STRING "" FORCE)
set(CMAKE_CXX_FLAGS "${ARCH_FLAGS} ${OPT_FLAGS}" CACHE STRING "" FORCE)

set(CMAKE_EXE_LINKER_FLAGS    "-Wl,-O1 -Wl,--as-needed" CACHE STRING "" FORCE)
set(CMAKE_SHARED_LINKER_FLAGS "-Wl,-O1 -Wl,--as-needed" CACHE STRING "" FORCE)

# 交叉编译标准设置：只在 sysroot 里找库和 header，但工具从 host 找
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)