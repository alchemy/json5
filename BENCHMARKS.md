# Benchmarks

Decoding performance of `json5` compared with the standard library's
`encoding/json` on the same inputs (all inputs are valid JSON, so both
packages can decode them). Targets are `any`.

## Environment

- Date: 2026-11-25
- Commit: `f49ff1d`
- Go: go1.27.4 linux/amd64
- CPU: Intel(R) Core(TM) Ultra X7 358H (16 threads)

## How to reproduce

```sh
go test -run '^$' -bench . -benchmem -count 6 ./...
```

Values below are the median of 6 runs. Run-to-run spread of ns/op was
4–20% (largest on `DecoderStream` and the `Std*Large*` cases), so treat
differences under ~10% as noise. Allocation figures are from the first run.

## Inputs

| Input | Description |
|---|---|
| Small | `{"name":"test","value":42,"active":true}` |
| Medium | ~4 KB flat object, 100 string fields |
| Large | ~100 KB flat object, 2000 string fields |
| DeeplyNested | 100 levels of nested `{"a": …}` objects |
| StringHeavy | object with 200 string fields containing escape sequences |
| LargeString | a single ~1 MB string value |
| Stream | 100 copies of Small, newline-separated, decoded one by one with a `Decoder` |

## Unmarshal

| Input | json5 time | encoding/json time | json5 / std | json5 MB/s | std MB/s | json5 B/op | std B/op | json5 allocs | std allocs |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| Small | 1.2 µs | 1.8 µs | 0.67× | 34.0 | 22.8 | 448 B | 448 B | 15 | 12 |
| Medium | 43.9 µs | 52.0 µs | 0.84× | 59.2 | 50.0 | 16.6 KB | 14.2 KB | 612 | 415 |
| Large | 1.71 ms | 1.47 ms | 1.17× | 80.5 | 94.2 | 758.2 KB | 533.8 KB | 16037 | 8039 |
| DeeplyNested | 40.9 µs | 69.0 µs | 0.59× | 14.7 | 8.7 | 34.4 KB | 39.7 KB | 302 | 511 |
| StringHeavy | 111.9 µs | 151.1 µs | 0.74× | 96.5 | 71.5 | 42.9 KB | 36.6 KB | 1414 | 817 |
| LargeString | 7.84 ms | 1.98 ms | 3.95× | 127.6 | 504.4 | 6.29 MB | 1.02 MB | 38 | 4 |

## Decoder

| Input | json5 time | encoding/json time | json5 / std | json5 MB/s | std MB/s | json5 B/op | std B/op | json5 allocs | std allocs |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| Small | 4.5 µs | 2.5 µs | 1.79× | 8.9 | 15.9 | 8.7 KB | 937 B | 19 | 17 |
| Medium | 69.3 µs | 62.9 µs | 1.10× | 37.5 | 41.4 | 23.4 KB | 22.8 KB | 615 | 426 |
| Large | 2.60 ms | 1.75 ms | 1.49× | 53.0 | 78.8 | 906.9 KB | 1.06 MB | 16076 | 8058 |
| DeeplyNested | 47.2 µs | 75.5 µs | 0.62× | 12.7 | 8.0 | 39.2 KB | 42.1 KB | 305 | 520 |
| StringHeavy | 200.6 µs | 190.4 µs | 1.05× | 53.9 | 56.7 | 64.7 KB | 69.7 KB | 1420 | 830 |
| LargeString | 12.92 ms | 4.20 ms | 3.07× | 77.4 | 237.9 | 12.53 MB | 3.13 MB | 301 | 25 |
| Stream | 240.3 µs | 196.9 µs | 1.22× | 17.1 | 20.8 | 61.3 KB | 53.4 KB | 1506 | 1212 |

## Observations

- `Unmarshal` is faster than `encoding/json` on small, medium, deeply
  nested and string-heavy inputs (roughly 0.6–0.85× the time), and about
  17% slower on the 100 KB object, where it makes about twice as many
  allocations.
- `LargeString` is the clear outlier: `json5` takes about 4× as long as
  `encoding/json` with `Unmarshal` and about 3× with `Decoder`, and
  allocates about 6× and 4× as many bytes. This is the most promising
  place to optimize.
- The streaming `Decoder` is slower than `json5.Unmarshal` on every input
  and generally on par with or slower than `encoding/json`'s `Decoder`;
  on `Small` it allocates 8.7 KB/op versus 0.9 KB/op for the standard library.

<details>
<summary>Raw output</summary>

```
goos: linux
goarch: amd64
pkg: github.com/alchemy/json5
cpu: Intel(R) Core(TM) Ultra X7 358H
BenchmarkUnmarshalSmall-16              	 1000000	      1220 ns/op	  32.77 MB/s	     448 B/op	      15 allocs/op
BenchmarkUnmarshalSmall-16              	 1052941	      1144 ns/op	  34.95 MB/s	     448 B/op	      15 allocs/op
BenchmarkUnmarshalSmall-16              	  954582	      1143 ns/op	  34.99 MB/s	     448 B/op	      15 allocs/op
BenchmarkUnmarshalSmall-16              	 1021551	      1158 ns/op	  34.53 MB/s	     448 B/op	      15 allocs/op
BenchmarkUnmarshalSmall-16              	  929056	      1196 ns/op	  33.45 MB/s	     448 B/op	      15 allocs/op
BenchmarkUnmarshalSmall-16              	 1069183	      1192 ns/op	  33.55 MB/s	     448 B/op	      15 allocs/op
BenchmarkUnmarshalMedium-16             	   27220	     43908 ns/op	  59.26 MB/s	   16595 B/op	     612 allocs/op
BenchmarkUnmarshalMedium-16             	   27996	     43938 ns/op	  59.22 MB/s	   16595 B/op	     612 allocs/op
BenchmarkUnmarshalMedium-16             	   27922	     44471 ns/op	  58.51 MB/s	   16595 B/op	     612 allocs/op
BenchmarkUnmarshalMedium-16             	   28368	     42817 ns/op	  60.77 MB/s	   16595 B/op	     612 allocs/op
BenchmarkUnmarshalMedium-16             	   26294	     42353 ns/op	  61.44 MB/s	   16595 B/op	     612 allocs/op
BenchmarkUnmarshalMedium-16             	   28706	     45734 ns/op	  56.89 MB/s	   16595 B/op	     612 allocs/op
BenchmarkUnmarshalLarge-16              	     697	   1770400 ns/op	  77.95 MB/s	  758157 B/op	   16037 allocs/op
BenchmarkUnmarshalLarge-16              	     696	   1708719 ns/op	  80.76 MB/s	  758160 B/op	   16037 allocs/op
BenchmarkUnmarshalLarge-16              	     709	   1676931 ns/op	  82.29 MB/s	  758137 B/op	   16037 allocs/op
BenchmarkUnmarshalLarge-16              	     756	   1717969 ns/op	  80.33 MB/s	  758061 B/op	   16037 allocs/op
BenchmarkUnmarshalLarge-16              	     735	   1745702 ns/op	  79.05 MB/s	  758095 B/op	   16037 allocs/op
BenchmarkUnmarshalLarge-16              	     670	   1694059 ns/op	  81.46 MB/s	  758205 B/op	   16037 allocs/op
BenchmarkUnmarshalDeeplyNested-16       	   30744	     40629 ns/op	  14.79 MB/s	   34447 B/op	     302 allocs/op
BenchmarkUnmarshalDeeplyNested-16       	   29964	     39525 ns/op	  15.21 MB/s	   34447 B/op	     302 allocs/op
BenchmarkUnmarshalDeeplyNested-16       	   31026	     41673 ns/op	  14.42 MB/s	   34447 B/op	     302 allocs/op
BenchmarkUnmarshalDeeplyNested-16       	   30170	     38868 ns/op	  15.46 MB/s	   34448 B/op	     302 allocs/op
BenchmarkUnmarshalDeeplyNested-16       	   27537	     42070 ns/op	  14.29 MB/s	   34448 B/op	     302 allocs/op
BenchmarkUnmarshalDeeplyNested-16       	   29044	     41197 ns/op	  14.59 MB/s	   34447 B/op	     302 allocs/op
BenchmarkUnmarshalStringHeavy-16        	   10054	    111683 ns/op	  96.72 MB/s	   42923 B/op	    1414 allocs/op
BenchmarkUnmarshalStringHeavy-16        	   10000	    113503 ns/op	  95.17 MB/s	   42923 B/op	    1414 allocs/op
BenchmarkUnmarshalStringHeavy-16        	    9511	    112140 ns/op	  96.33 MB/s	   42924 B/op	    1414 allocs/op
BenchmarkUnmarshalStringHeavy-16        	   10444	    116896 ns/op	  92.41 MB/s	   42923 B/op	    1414 allocs/op
BenchmarkUnmarshalStringHeavy-16        	    9594	    109931 ns/op	  98.26 MB/s	   42923 B/op	    1414 allocs/op
BenchmarkUnmarshalStringHeavy-16        	    9310	    110538 ns/op	  97.72 MB/s	   42924 B/op	    1414 allocs/op
BenchmarkUnmarshalLargeString-16        	     152	   7834085 ns/op	 127.65 MB/s	 6292759 B/op	      38 allocs/op
BenchmarkUnmarshalLargeString-16        	     152	   7839237 ns/op	 127.56 MB/s	 6292722 B/op	      38 allocs/op
BenchmarkUnmarshalLargeString-16        	     148	   8191933 ns/op	 122.07 MB/s	 6293838 B/op	      38 allocs/op
BenchmarkUnmarshalLargeString-16        	     152	   7811625 ns/op	 128.01 MB/s	 6292723 B/op	      38 allocs/op
BenchmarkUnmarshalLargeString-16        	     150	   7780142 ns/op	 128.53 MB/s	 6293258 B/op	      38 allocs/op
BenchmarkUnmarshalLargeString-16        	     142	   8046076 ns/op	 124.28 MB/s	 6295610 B/op	      38 allocs/op
BenchmarkDecoderSmall-16                	  249846	      4657 ns/op	   8.59 MB/s	    8736 B/op	      19 allocs/op
BenchmarkDecoderSmall-16                	  256827	      4634 ns/op	   8.63 MB/s	    8736 B/op	      19 allocs/op
BenchmarkDecoderSmall-16                	  247999	      4562 ns/op	   8.77 MB/s	    8736 B/op	      19 allocs/op
BenchmarkDecoderSmall-16                	  254701	      4462 ns/op	   8.97 MB/s	    8736 B/op	      19 allocs/op
BenchmarkDecoderSmall-16                	  256312	      4336 ns/op	   9.23 MB/s	    8736 B/op	      19 allocs/op
BenchmarkDecoderSmall-16                	  248720	      4435 ns/op	   9.02 MB/s	    8736 B/op	      19 allocs/op
BenchmarkDecoderMedium-16               	   17412	     66329 ns/op	  39.23 MB/s	   23416 B/op	     615 allocs/op
BenchmarkDecoderMedium-16               	   18030	     73738 ns/op	  35.29 MB/s	   23416 B/op	     615 allocs/op
BenchmarkDecoderMedium-16               	   17432	     73920 ns/op	  35.20 MB/s	   23416 B/op	     615 allocs/op
BenchmarkDecoderMedium-16               	   16527	     67253 ns/op	  38.69 MB/s	   23416 B/op	     615 allocs/op
BenchmarkDecoderMedium-16               	   16977	     69967 ns/op	  37.19 MB/s	   23416 B/op	     615 allocs/op
BenchmarkDecoderMedium-16               	   16315	     68693 ns/op	  37.88 MB/s	   23416 B/op	     615 allocs/op
BenchmarkDecoderLarge-16                	     480	   2579920 ns/op	  53.49 MB/s	  906917 B/op	   16076 allocs/op
BenchmarkDecoderLarge-16                	     448	   2664472 ns/op	  51.79 MB/s	  907044 B/op	   16076 allocs/op
BenchmarkDecoderLarge-16                	     403	   2555670 ns/op	  54.00 MB/s	  907259 B/op	   16077 allocs/op
BenchmarkDecoderLarge-16                	     435	   2677905 ns/op	  51.53 MB/s	  907102 B/op	   16077 allocs/op
BenchmarkDecoderLarge-16                	     477	   2557852 ns/op	  53.95 MB/s	  906930 B/op	   16076 allocs/op
BenchmarkDecoderLarge-16                	     456	   2623025 ns/op	  52.61 MB/s	  907013 B/op	   16076 allocs/op
BenchmarkDecoderDeeplyNested-16         	   26068	     48195 ns/op	  12.47 MB/s	   39208 B/op	     305 allocs/op
BenchmarkDecoderDeeplyNested-16         	   25114	     46420 ns/op	  12.95 MB/s	   39208 B/op	     305 allocs/op
BenchmarkDecoderDeeplyNested-16         	   25045	     47547 ns/op	  12.64 MB/s	   39208 B/op	     305 allocs/op
BenchmarkDecoderDeeplyNested-16         	   23960	     47830 ns/op	  12.57 MB/s	   39208 B/op	     305 allocs/op
BenchmarkDecoderDeeplyNested-16         	   26252	     45277 ns/op	  13.27 MB/s	   39208 B/op	     305 allocs/op
BenchmarkDecoderDeeplyNested-16         	   24694	     46762 ns/op	  12.85 MB/s	   39208 B/op	     305 allocs/op
BenchmarkDecoderStringHeavy-16          	    5750	    181322 ns/op	  59.57 MB/s	   64706 B/op	    1420 allocs/op
BenchmarkDecoderStringHeavy-16          	    9618	    199285 ns/op	  54.20 MB/s	   64702 B/op	    1420 allocs/op
BenchmarkDecoderStringHeavy-16          	    5694	    203358 ns/op	  53.12 MB/s	   64706 B/op	    1420 allocs/op
BenchmarkDecoderStringHeavy-16          	    5474	    201880 ns/op	  53.51 MB/s	   64707 B/op	    1420 allocs/op
BenchmarkDecoderStringHeavy-16          	    5163	    202783 ns/op	  53.27 MB/s	   64707 B/op	    1420 allocs/op
BenchmarkDecoderStringHeavy-16          	    6416	    196279 ns/op	  55.03 MB/s	   64705 B/op	    1420 allocs/op
BenchmarkDecoderLargeString-16          	      93	  12921060 ns/op	  77.39 MB/s	12528624 B/op	     301 allocs/op
BenchmarkDecoderLargeString-16          	      97	  12914470 ns/op	  77.43 MB/s	12525859 B/op	     301 allocs/op
BenchmarkDecoderLargeString-16          	      93	  13213313 ns/op	  75.68 MB/s	12528616 B/op	     301 allocs/op
BenchmarkDecoderLargeString-16          	      90	  12602527 ns/op	  79.35 MB/s	12530872 B/op	     301 allocs/op
BenchmarkDecoderLargeString-16          	      91	  12963438 ns/op	  77.14 MB/s	12530109 B/op	     301 allocs/op
BenchmarkDecoderLargeString-16          	      94	  12840661 ns/op	  77.88 MB/s	12527912 B/op	     301 allocs/op
BenchmarkDecoderStream-16               	    3766	    278437 ns/op	  14.72 MB/s	   61252 B/op	    1506 allocs/op
BenchmarkDecoderStream-16               	    4501	    243253 ns/op	  16.85 MB/s	   61251 B/op	    1506 allocs/op
BenchmarkDecoderStream-16               	    4893	    231056 ns/op	  17.74 MB/s	   61251 B/op	    1506 allocs/op
BenchmarkDecoderStream-16               	    5215	    237363 ns/op	  17.27 MB/s	   61251 B/op	    1506 allocs/op
BenchmarkDecoderStream-16               	    5218	    248828 ns/op	  16.47 MB/s	   61251 B/op	    1506 allocs/op
BenchmarkDecoderStream-16               	    5106	    236658 ns/op	  17.32 MB/s	   61251 B/op	    1506 allocs/op
BenchmarkStdUnmarshalSmall-16           	  622296	      1675 ns/op	  23.89 MB/s	     448 B/op	      12 allocs/op
BenchmarkStdUnmarshalSmall-16           	  695103	      1709 ns/op	  23.41 MB/s	     448 B/op	      12 allocs/op
BenchmarkStdUnmarshalSmall-16           	  592232	      1801 ns/op	  22.21 MB/s	     448 B/op	      12 allocs/op
BenchmarkStdUnmarshalSmall-16           	  629481	      1719 ns/op	  23.27 MB/s	     448 B/op	      12 allocs/op
BenchmarkStdUnmarshalSmall-16           	  615741	      1788 ns/op	  22.38 MB/s	     448 B/op	      12 allocs/op
BenchmarkStdUnmarshalSmall-16           	  657492	      1800 ns/op	  22.23 MB/s	     448 B/op	      12 allocs/op
BenchmarkStdUnmarshalMedium-16          	   22411	     55180 ns/op	  47.16 MB/s	   14235 B/op	     415 allocs/op
BenchmarkStdUnmarshalMedium-16          	   22750	     51744 ns/op	  50.29 MB/s	   14235 B/op	     415 allocs/op
BenchmarkStdUnmarshalMedium-16          	   21812	     52899 ns/op	  49.19 MB/s	   14234 B/op	     415 allocs/op
BenchmarkStdUnmarshalMedium-16          	   22423	     52287 ns/op	  49.76 MB/s	   14235 B/op	     415 allocs/op
BenchmarkStdUnmarshalMedium-16          	   23994	     51576 ns/op	  50.45 MB/s	   14235 B/op	     415 allocs/op
BenchmarkStdUnmarshalMedium-16          	   23532	     49961 ns/op	  52.08 MB/s	   14236 B/op	     415 allocs/op
BenchmarkStdUnmarshalLarge-16           	     828	   1420176 ns/op	  97.17 MB/s	  533815 B/op	    8039 allocs/op
BenchmarkStdUnmarshalLarge-16           	     777	   1470665 ns/op	  93.84 MB/s	  533882 B/op	    8039 allocs/op
BenchmarkStdUnmarshalLarge-16           	     896	   1460677 ns/op	  94.48 MB/s	  533736 B/op	    8039 allocs/op
BenchmarkStdUnmarshalLarge-16           	     896	   1330671 ns/op	 103.71 MB/s	  533744 B/op	    8039 allocs/op
BenchmarkStdUnmarshalLarge-16           	     907	   1488998 ns/op	  92.68 MB/s	  533724 B/op	    8039 allocs/op
BenchmarkStdUnmarshalLarge-16           	     775	   1576490 ns/op	  87.54 MB/s	  533887 B/op	    8039 allocs/op
BenchmarkStdUnmarshalDeeplyNested-16    	   18472	     68866 ns/op	   8.73 MB/s	   39705 B/op	     511 allocs/op
BenchmarkStdUnmarshalDeeplyNested-16    	   15748	     77799 ns/op	   7.73 MB/s	   39707 B/op	     511 allocs/op
BenchmarkStdUnmarshalDeeplyNested-16    	   16196	     74742 ns/op	   8.04 MB/s	   39706 B/op	     511 allocs/op
BenchmarkStdUnmarshalDeeplyNested-16    	   15776	     66348 ns/op	   9.06 MB/s	   39704 B/op	     511 allocs/op
BenchmarkStdUnmarshalDeeplyNested-16    	   18658	     69057 ns/op	   8.70 MB/s	   39703 B/op	     511 allocs/op
BenchmarkStdUnmarshalDeeplyNested-16    	   17575	     68337 ns/op	   8.79 MB/s	   39703 B/op	     511 allocs/op
BenchmarkStdUnmarshalStringHeavy-16     	    7718	    152270 ns/op	  70.94 MB/s	   36562 B/op	     817 allocs/op
BenchmarkStdUnmarshalStringHeavy-16     	    7268	    149978 ns/op	  72.02 MB/s	   36562 B/op	     817 allocs/op
BenchmarkStdUnmarshalStringHeavy-16     	    7988	    152634 ns/op	  70.77 MB/s	   36563 B/op	     817 allocs/op
BenchmarkStdUnmarshalStringHeavy-16     	    7380	    147695 ns/op	  73.14 MB/s	   36562 B/op	     817 allocs/op
BenchmarkStdUnmarshalStringHeavy-16     	    6745	    155587 ns/op	  69.43 MB/s	   36563 B/op	     817 allocs/op
BenchmarkStdUnmarshalStringHeavy-16     	    7488	    148465 ns/op	  72.76 MB/s	   36562 B/op	     817 allocs/op
BenchmarkStdUnmarshalLargeString-16     	     774	   1801496 ns/op	 555.10 MB/s	 1016539 B/op	       4 allocs/op
BenchmarkStdUnmarshalLargeString-16     	     583	   1986884 ns/op	 503.30 MB/s	 1019210 B/op	       4 allocs/op
BenchmarkStdUnmarshalLargeString-16     	     571	   1978205 ns/op	 505.51 MB/s	 1019409 B/op	       4 allocs/op
BenchmarkStdUnmarshalLargeString-16     	     614	   2001872 ns/op	 499.53 MB/s	 1018698 B/op	       4 allocs/op
BenchmarkStdUnmarshalLargeString-16     	     540	   2060403 ns/op	 485.34 MB/s	 1020051 B/op	       4 allocs/op
BenchmarkStdUnmarshalLargeString-16     	     691	   1966625 ns/op	 508.49 MB/s	 1017529 B/op	       4 allocs/op
BenchmarkStdDecoderSmall-16             	  402382	      2513 ns/op	  15.92 MB/s	     937 B/op	      17 allocs/op
BenchmarkStdDecoderSmall-16             	  407958	      2518 ns/op	  15.88 MB/s	     937 B/op	      17 allocs/op
BenchmarkStdDecoderSmall-16             	  449805	      2351 ns/op	  17.01 MB/s	     937 B/op	      17 allocs/op
BenchmarkStdDecoderSmall-16             	  483332	      2521 ns/op	  15.87 MB/s	     937 B/op	      17 allocs/op
BenchmarkStdDecoderSmall-16             	  499322	      2471 ns/op	  16.19 MB/s	     937 B/op	      17 allocs/op
BenchmarkStdDecoderSmall-16             	  449481	      2551 ns/op	  15.68 MB/s	     937 B/op	      17 allocs/op
BenchmarkStdDecoderMedium-16            	   19268	     63256 ns/op	  41.13 MB/s	   22807 B/op	     426 allocs/op
BenchmarkStdDecoderMedium-16            	   18092	     62458 ns/op	  41.66 MB/s	   22807 B/op	     426 allocs/op
BenchmarkStdDecoderMedium-16            	   18598	     62192 ns/op	  41.84 MB/s	   22807 B/op	     426 allocs/op
BenchmarkStdDecoderMedium-16            	   19658	     63324 ns/op	  41.09 MB/s	   22808 B/op	     426 allocs/op
BenchmarkStdDecoderMedium-16            	   18045	     62134 ns/op	  41.88 MB/s	   22807 B/op	     426 allocs/op
BenchmarkStdDecoderMedium-16            	   18645	     64550 ns/op	  40.31 MB/s	   22806 B/op	     426 allocs/op
BenchmarkStdDecoderLarge-16             	     690	   1816032 ns/op	  75.99 MB/s	 1060099 B/op	    8058 allocs/op
BenchmarkStdDecoderLarge-16             	     697	   1738370 ns/op	  79.39 MB/s	 1060040 B/op	    8058 allocs/op
BenchmarkStdDecoderLarge-16             	     608	   1719327 ns/op	  80.27 MB/s	 1060240 B/op	    8059 allocs/op
BenchmarkStdDecoderLarge-16             	     660	   1702891 ns/op	  81.04 MB/s	 1060147 B/op	    8058 allocs/op
BenchmarkStdDecoderLarge-16             	     714	   1810035 ns/op	  76.24 MB/s	 1060019 B/op	    8058 allocs/op
BenchmarkStdDecoderLarge-16             	     652	   1762706 ns/op	  78.29 MB/s	 1060335 B/op	    8059 allocs/op
BenchmarkStdDecoderDeeplyNested-16      	   16051	     75246 ns/op	   7.99 MB/s	   42123 B/op	     520 allocs/op
BenchmarkStdDecoderDeeplyNested-16      	   15850	     76438 ns/op	   7.86 MB/s	   42127 B/op	     520 allocs/op
BenchmarkStdDecoderDeeplyNested-16      	   16185	     75478 ns/op	   7.96 MB/s	   42125 B/op	     520 allocs/op
BenchmarkStdDecoderDeeplyNested-16      	   15441	     73764 ns/op	   8.15 MB/s	   42128 B/op	     520 allocs/op
BenchmarkStdDecoderDeeplyNested-16      	   15750	     75453 ns/op	   7.97 MB/s	   42126 B/op	     520 allocs/op
BenchmarkStdDecoderDeeplyNested-16      	   15727	     76929 ns/op	   7.81 MB/s	   42120 B/op	     520 allocs/op
BenchmarkStdDecoderStringHeavy-16       	    7642	    204518 ns/op	  52.82 MB/s	   69739 B/op	     830 allocs/op
BenchmarkStdDecoderStringHeavy-16       	    5971	    187692 ns/op	  57.55 MB/s	   69746 B/op	     830 allocs/op
BenchmarkStdDecoderStringHeavy-16       	    5415	    190097 ns/op	  56.82 MB/s	   69732 B/op	     830 allocs/op
BenchmarkStdDecoderStringHeavy-16       	    6090	    189936 ns/op	  56.87 MB/s	   69740 B/op	     830 allocs/op
BenchmarkStdDecoderStringHeavy-16       	    5769	    190928 ns/op	  56.58 MB/s	   69741 B/op	     830 allocs/op
BenchmarkStdDecoderStringHeavy-16       	    5865	    190683 ns/op	  56.65 MB/s	   69732 B/op	     830 allocs/op
BenchmarkStdDecoderLargeString-16       	     322	   4467953 ns/op	 223.82 MB/s	 3130048 B/op	      25 allocs/op
BenchmarkStdDecoderLargeString-16       	     274	   4127256 ns/op	 242.29 MB/s	 3133642 B/op	      25 allocs/op
BenchmarkStdDecoderLargeString-16       	     286	   4307228 ns/op	 232.17 MB/s	 3132684 B/op	      25 allocs/op
BenchmarkStdDecoderLargeString-16       	     284	   4067355 ns/op	 245.86 MB/s	 3132812 B/op	      25 allocs/op
BenchmarkStdDecoderLargeString-16       	     319	   4282017 ns/op	 233.54 MB/s	 3130458 B/op	      25 allocs/op
BenchmarkStdDecoderLargeString-16       	     286	   3735178 ns/op	 267.73 MB/s	 3132818 B/op	      25 allocs/op
BenchmarkStdDecoderStream-16            	    6386	    198946 ns/op	  20.60 MB/s	   53424 B/op	    1212 allocs/op
BenchmarkStdDecoderStream-16            	    5593	    198086 ns/op	  20.69 MB/s	   53420 B/op	    1212 allocs/op
BenchmarkStdDecoderStream-16            	    5554	    199410 ns/op	  20.56 MB/s	   53416 B/op	    1212 allocs/op
BenchmarkStdDecoderStream-16            	    6044	    192121 ns/op	  21.34 MB/s	   53420 B/op	    1212 allocs/op
BenchmarkStdDecoderStream-16            	    5703	    194433 ns/op	  21.08 MB/s	   53419 B/op	    1212 allocs/op
BenchmarkStdDecoderStream-16            	    5938	    195720 ns/op	  20.94 MB/s	   53430 B/op	    1212 allocs/op
PASS
ok  	github.com/alchemy/json5	247.498s
```

</details>
