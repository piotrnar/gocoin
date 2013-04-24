TurboDB
======

Fast persistent sorage database.

Must be used from a single thread.

===
database.0 file format:
1) 32-but KeySize LSB
2 ... N-1)
 * KeySize bytes - Key
 * 32-bits LSB - Value Length
 * Value Length bytes - Value
N)
 * FFFFFFFF - Marker
 * 32-bits LSB - DB version sequence
 * "FINI"

===
database.log file format:
32-bits LSB - DB version sequence that this log file belongs to
N records:
 * 1 byte: 0-delete, 1-add
 * KeySize bytes - Key
 * If add:
    * 32-bits LSB - Value Length
    * Value Length bytes - Value
 * crc32 of the record (including the first byte)