#Glamscan

A quick and (very) dirty wrapper around clamAV that does batch submits of files in a given directory to check for viruses. It uses the clamAV tcp interface and automatically deletes viruses that it finds.

For usage instructions: `./glamscan -h`.

#Building

This project uses glide to manage dependencies, once it's installed, run `glide install` in the top directory followed by `go build`.

#Testing

You can easily run clamAV locally with docker:

    docker run -d -p 3310:3310 mkodockx/docker-clamav

Build the binary and point it at the directory you want to scan:

    ./glamscan -address localhost -directory .

If you want to test out the virus profiling, use an EICAR test file (string split in README so that this doesn't get picked up as a virus):

   echo -n 'X5O!P%@AP[4\PZX54(P^)7CC)7}$' > eicar.txt && echo 'EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*' >> eicar.txt
