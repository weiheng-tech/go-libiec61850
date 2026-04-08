set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR arm)
set(CMAKE_C_FLAGS "-O2 -march=armv7-a -mfpu=neon-vfpv4 -mfloat-abi=hard -DNDEBUG")
set(CMAKE_C_COMPILER /usr/bin/arm-linux-gnueabihf-gcc)