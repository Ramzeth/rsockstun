param (
    [string]$server = "localhost", 
    [int]$port = 8888,    
    [string]$pass = "hello"
)

$buffer = New-Object System.Byte[] 2048
[void][byte[]] $ymxbuffer 
[void][byte[]] $databuffer 
$tmpbuffer = New-Object System.Byte[] 12
$wrnum = 0
$bufsize = 8192


[ScriptBlock]$socksScript = {
    #do socks server
    param($state)

    function Get-IpAddress{
        param($ip)
        IF ($ip -as [ipaddress]){
            return $ip
        }else{
            $ip2 = [System.Net.Dns]::GetHostAddresses($ip)[0].IPAddressToString;
        }
        return $ip2
    }

    $buffer = New-Object System.Byte[] 1500
    
    
    #"Debug : inside socks thread. StreamID: " | out-file -Append c:\work\log.txt
    #$state.StreamID | out-file -Append c:\work\log.txt
    
    try{
        
        #get socks ver
        $state.inputStream.Read($buffer,0,2) | Out-Null
        $socksVer=$buffer[0]
        if ($socksVer -eq 5){
            #"Debug : inside socks thread. Socksver = 5 " | out-file -Append c:\work\log.txt
            $state.inputStream.Read($buffer,2,$buffer[1]) | Out-Null
            for ($i=2; $i -le $buffer[1]+1; $i++) {
                if ($buffer[$i] -eq 0) {break}
            }
            if ($buffer[$i] -ne 0){
                $buffer[1]=255
                $state.outputStream.Write($buffer,0,2)
            }else{
                $buffer[1]=0
                $state.outputStream.Write($buffer,0,2)
            }
            $state.inputStream.Read($buffer,0,4) | Out-Null
            $cmd = $buffer[1]
            $atyp = $buffer[3]
            if($cmd -ne 1){
                $buffer[1] = 7
                $state.outputStream.Write($buffer,0,2)
                #"Debug : inside socks thread. Not a connect: " | out-file -Append c:\work\log.txt
                $state.outputStream.Write($buffer,0,2)
                $StopFlag[$state.StreamID] = 2
                throw "Not a connect"
            }
            if($atyp -eq 1){
                $ipv4 = New-Object System.Byte[] 4
                $state.inputStream.Read($ipv4,0,4) | Out-Null
                $ipAddress = New-Object System.Net.IPAddress(,$ipv4)
                $hostName = $ipAddress.ToString()
                #"Debug : inside socks thread. ipaddress: $ipaddress, hostname: $hostname " | out-file -Append c:\work\log.txt
            }elseif($atyp -eq 3){
                $state.inputStream.Read($buffer,4,1) | Out-Null
                $hostBuff = New-Object System.Byte[] $buffer[4]
                $state.inputStream.Read($hostBuff,0,$buffer[4]) | Out-Null
                $hostName = [System.Text.Encoding]::ASCII.GetString($hostBuff)
                #"Debug : inside socks thread.  hostname: $hostname " | out-file -Append c:\work\log.txt
            }
            else{
                $buffer[1] = 8
                $state.outputStream.Write($buffer,0,2)
                #"Debug : inside socks thread. Not a valid destination address " | out-file -Append c:\work\log.txt
                $buffer[1]=4
                $state.outputStream.Write($buffer,0,2)
                $StopFlag[$state.StreamID] = 2
                throw "Not a valid destination address"
            }
            $state.inputStream.Read($buffer,4,2) | Out-Null
            $destPort = $buffer[4]*256 + $buffer[5]
            #"Debug : inside socks thread. Call IP address func. Hostname: $hostname " | out-file -Append c:\work\log.txt
            $destHost = Get-IpAddress($hostName)

            #"Debug : inside socks thread. Done. DestHost: $DestHost " | out-file -Append c:\work\log.txt
            if($destHost -eq $null){
                $buffer[1]=4
                $state.outputStream.Write($buffer,0,2)
                #"Debug : inside socks thread. Cant resolve destination address " | out-file -Append c:\work\log.txt
                throw "Cant resolve destination address"
            }

            #"Debug : inside socks thread. Connecting to... $desthost, $destport " | out-file -Append c:\work\log.txt
            
            $tmpServ = New-Object System.Net.Sockets.TcpClient($destHost, $destPort)
            
            #$tmpServ = New-Object System.Net.Sockets.TcpClient('127.0.0.1', 8889)
            if($tmpServ.Connected){
                #"Debug : inside socks thread. $state.StreamID connected " | out-file -Append c:\work\log.txt
                
                $buffer[1]=0
                $buffer[3]=1
                $buffer[4]=0
                $buffer[5]=0
                $state.outputStream.Write($buffer,0,10)
                $state.outputStream.Flush()



                $srvStream = $tmpServ.GetStream()
                #$AsyncJobResult2 = $srvStream.WriteAsync([byte[]][char[]]"GET / HTTP/1.1`n`n",0,16)
                $AsyncJobResult2 = $state.inputStream.CopyToAsync($srvstream)
                #"Debug : inside thread  $state.StreamID . state.inputStream.CopyToAsync done " | out-file -Append c:\work\log.txt
                $AsyncJobResult = $srvStream.CopyToAsync($state.outputStream)
                #"Debug : inside thread $state.StreamID . srvStream.CopyToAsync done " | out-file -Append c:\work\log.txt
                #$AsyncJobResult.AsyncWaitHandle.WaitOne();
                #$AsyncJobResult2.AsyncWaitHandle.WaitOne();
                while ($StopFlag[$state.StreamID] -eq 0 -and $tmpServ.Connected ){
                    #"Debug : inside thread. EndFlag is NOT set " | out-file -Append c:\work\log.txt
                    #$StopFlag[$state.StreamID]  | out-file -Append c:\work\log.txt
                    start-sleep -Milliseconds 5
                }

            
                if ($tmpServ.Connected){
                    #"Debug : inside thread. finishing by yamux... " | out-file -Append c:\work\log.txt
                    $tmpServ.close()
                }else{
                    #"Debug : inside thread. finishing by socks... " | out-file -Append c:\work\log.txt
                    $StopFlag[$state.StreamID] = 2
                }

            }else{
                #"Debug : inside thread. Can not connect " | out-file -Append c:\work\log.txt
                $buffer[1]=4
                $state.outputStream.Write($buffer,0,2)
                $StopFlag[$state.StreamID] = 2
                throw "thread extension: can not a connect"
            
                }
            
            }
            
        }
    catch{
         $StopFlag[$state.StreamID] = 2
         throw "thread extension: some error"
    }
    finaly{
        if ($tmpServ -ne $null) {
            $tmpServ.Dispose()
        }
        #"Debug : inside socks thread. Closing thread. StreamID: " | out-file -Append c:\work\log.txt
        #$state.StreamID | out-file -Append c:\work\log.txt

        Exit;
    }
}


[ScriptBlock]$yamuxScript = {
    #going walktrough a streams list, quering them and puting a results to yamux with corresponding yamux stream id
    param($state)
    #"Debug : inside yamux thread. Hello ! " | out-file -Append c:\work\log.txt
    while($true){
        #"Debug : inside yamux thread. Hello ! " | out-file -Append c:\work\log.txt
        foreach ($stream in $state.streams){
            #"Debug : inside yamux thread. working with stream: " | out-file -Append c:\work\log.txt
            #$stream.ymxId | out-file -Append c:\work\log.txt

            if ($stream.readjob -eq $null){
                $stream.readjob = $stream.sinputStream.ReadAsync($stream.readbuffer,0,$state.bufsize)
            }elseif ( $stream.readjob.IsCompleted  ){
                #if read asyncjob completed 
                #"Debug : inside yamux thread. got data from socks thread " | out-file -Append c:\work\log.txt
                #$stream.ymxId | out-file -Append c:\work\log.txt
	            $outbuf = [byte[]](0x00,0x00,0x00,0x00)+ [bitconverter]::getbytes([int32]$stream.ymxId)[3..0]+ [bitconverter]::getbytes([int32]$stream.readjob.Result)[3..0]
                #"Debug : inside yamux thread. writing YMX 12 byte DATA header to tcpstream " | out-file -Append c:\work\log.txt
	            $state.tcpstream.Write($outbuf,0,12)
            
                #write raw data from socks thread to yamux
                #"Debug : inside yamux thread. writing raw DATA header to tcpstream " | out-file -Append c:\work\log.txt
                $state.tcpstream.Write($stream.readbuffer,0,$stream.readjob.Result)
                $state.tcpstream.flush()

                #create new readasync job
                $stream.readjob = $stream.sinputStream.ReadAsync($stream.readbuffer,0,$state.bufsize)
            }else{
                #write-host "Not readed"
            }


            #$stopflag | out-file -Append c:\work\log.txt 

            #check StopFlag for FYN from socks
            if ($StopFlag[$stream.ymxId] -eq 2){
                #"Debug : inside yamux thread. got STOP flag from socks " | out-file -Append c:\work\log.txt
                #$stream.ymxId | out-file -Append c:\work\log.txt
                $StopFlag[$stream.ymxId] = 3
                $outbuf = [byte[]](0x00,0x01,0x00,0x04)+ [bitconverter]::getbytes([int32]$stream.ymxId)[3..0]+ [byte[]](0x00,0x00,0x00,0x00)
                $state.tcpstream.Write($outbuf,0,12)
                $state.tcpstream.flush()
            }

            #check RcvBytes for 256K size
            if ($RcvBytes[$stream.ymxId] -ge 256144){
                #out win update ymx packet with 256K size 
                $outbuf = [byte[]](0x00,0x01,0x00,0x00)+ [bitconverter]::getbytes([int32]$stream.ymxId)[3..0]+ (0x00,0x04,0x00,0x00)
	            $state.tcpstream.Write($outbuf,0,12)
                $RcvBytes[$stream.ymxId] = 0
            }

        }
        start-sleep -Milliseconds 5
    }
}

#"Debug : begin" | out-file c:\work\log.txt
[System.Collections.ArrayList]$streams = @{}
$StopFlag = [hashtable]::Synchronized(@{})#Shared array of flags. 0 - normal;1 - got FIN from yamux; 2 - got FIN from socks; 3 - got FIN from socks and FIN packet was sent to yamux
$RcvBytes = [hashtable]::Synchronized(@{})#Shared array of num of received bytes. When its eq or more than 256144 - then we have to send ymx window update packet, otherwise files > 256k will not be transfered by yamux

$tcpConnection = New-Object System.Net.Sockets.TcpClient($server, $port)
#$tcpStream = $tcpConnection.GetStream() #uncomment for cleartext connection
#comment 2 lines for cleartext connection
$tcpStream = New-Object System.Net.Security.SslStream($tcpConnection.GetStream(),$false,({$True} -as [Net.Security.RemoteCertificateValidationCallback]))
$tcpStream.AuthenticateAsClient('127.0.0.1')

if ($tcpConnection.Connected)
{
 write-host "conneccted"
    $tcpstream.Write([byte[]][char[]]$pass,0,$pass.length)

    #create runspase for shared arrays StopFlag, RcvBytes
    $ymxrunspace = [runspacefactory]::CreateRunspace()
    $ymxrunspace.Open()
    $ymxrunspace.SessionStateProxy.SetVariable("StopFlag",$StopFlag)
    $ymxrunspace.SessionStateProxy.SetVariable("RcvBytes",$RcvBytes)

    #start async yamux stript(thread) -  
    write-host "Debug: staring yamux thread"
    $state = [PSCustomObject]@{"streams"=$streams;"tcpStream"=$tcpstream;"bufsize"=$bufsize}
    $PS1 = [PowerShell]::Create()
    $PS1.Runspace = $ymxrunspace
    $PS1.AddScript($yamuxScript).AddArgument($state) | Out-Null
    [System.IAsyncResult]$AsyncJobResult = $null
    $ymxAsyncJob = $PS1.BeginInvoke()
        
    while($tcpConnection.Connected){
        $ymxbuffer = $null
        $tnum = 0 
        #read 12 bytes of ymx header; we have to use cycle, because there may be multiple read attempts...
        do {
            try { $num = $tcpStream.Read($tmpbuffer,0,12) } catch {}
            $tnum += $num
            $ymxbuffer += $tmpbuffer[0..($num-1)]
        }while ($tnum -lt 12 -and $tcpConnection.Connected)
        
        #write-host $ymxbuffer
	 if ($ymxbuffer[1] -eq 1 -and $ymxbuffer[3] -eq 1){
	    write-host "Debug: got ymx SYN"
	    $ymxstream = [bitconverter]::ToInt32($ymxbuffer[7..4],0)
	    #write-host "Yamux stream ID: $ymxstream"

	    #we do not need send reply for ymx SYN, but send it...
	    $outbuf = [byte[]](0x00,0x01,0x00,0x02,$ymxstream[3],$ymxstream[2],$ymxstream[1],$ymxstream[0],0x00,0x00,0x00,0x00)
	    $tcpstream.Write($outbuf,0,12)

	    #create and start thread
	    #write-host "Debug: creating thread"
        #create 2 pipes: IN and OUT. Create 4 streams: Server and Client stream for every pipe 
	    $sipipe = new-object System.IO.Pipes.AnonymousPipeServerStream(1)
        $sopipe = new-object System.IO.Pipes.AnonymousPipeServerStream(2,1)
        $sipipe_clHandle = $sipipe.GetClientHandleAsString()
        $sopipe_clHandle = $sopipe.GetClientHandleAsString()
        $cipipe = new-object System.IO.Pipes.AnonymousPipeClientStream(1,$sopipe_clHandle)
        $copipe = new-object System.IO.Pipes.AnonymousPipeClientStream(2,$sipipe_clHandle)
        
        $readbuffer = New-Object System.Byte[] $bufsize

        $state = [PSCustomObject]@{"StreamID"=$ymxstream;"inputStream"=$cipipe;"outputStream"=$copipe}
        $PS = [PowerShell]::Create()

        #create runspase for shared variable StopFlag
        $socksrunspace = [runspacefactory]::CreateRunspace()
        $socksrunspace.Open()
        $socksrunspace.SessionStateProxy.SetVariable("StopFlag",$StopFlag)
        $PS.Runspace = $socksrunspace
        $PS.AddScript($socksScript).AddArgument($state) | Out-Null
        [System.IAsyncResult]$AsyncJobResult = $null
        $StopFlag[$ymxstream] = 0
        $RcvBytes[$ymxstream] = 0
        $AsyncJobResult = $PS.BeginInvoke()

        #add created streams and PS object to streams list
        $streams.add(@{ymxId=$ymxstream;cinputStream=$cipipe;sinputStream=$sipipe;coutputStream=$copipe;soutputStream=$sopipe;asyncobj=$AsyncJobResult;psobj=$PS;readjob=$null;readbuffer=$readbuffer}) | out-null
        $readbuffer.clear()

	 }elseif ($ymxbuffer[1] -eq 2){
	 	#write-host "got ymx keepalive"
	 	$pingval = $ymxbuffer[8..11]
	 	$outbuf = [byte[]](0x00,0x02,0x00,0x02,0x00,0x00,0x00,0x00,$pingval[0],$pingval[1],$pingval[2],$pingval[3])
	 	$tcpstream.Write($outbuf,0,12)

        
	 }elseif ($ymxbuffer[1] -eq 0) {
        #write-host "Debug: got ymx DATA"
	    $ymxstream = [bitconverter]::ToInt32($ymxbuffer[7..4],0)
        $ymxcount = [bitconverter]::ToInt32($ymxbuffer[11..8],0)
	    
        #search in streamlist inpsteam and outstream by ymx Id and get streams from streams list
        if ($streams.Count -gt 1){$streamind = $streams.ymxId.IndexOf($ymxstream)} 
        else {$streamind = 0}
        $inpStream = $streams[$streamind].sinputStream
        $outStream = $streams[$streamind].soutputStream
        if($inpStream -and $outStream ){
            #read raw data
            #write-host "Debug: read raw ymx DATA"

            $databuffer = $null
            $tnum = 0 
            do {
                if ($buffer.length -le ($ymxcount-$tnum)) { $num = $tcpStream.Read($buffer,0,$buffer.Length) }else
                { $num = $tcpStream.Read($buffer,0,($ymxcount-$tnum)) }
                $tnum += $num
                $databuffer += $buffer[0..($num-1)]
            }while ($tnum -lt $ymxcount -and $tcpConnection.Connected)
            
            #write-host "out " $ymxcount" bytes to outstream at index:" $streamind
            $outStream.Write($databuffer,0,$ymxcount)

            #update $RcvBytes
            $RcvBytes[$ymxstream] += $ymxcount
        }else{
            write-host "Debug: can't find streams. StreamId: $streamid, inpStream: $inpStream, outstream: $outStream"
        }


     }elseif ($ymxbuffer[1] -eq 1 -and $ymxbuffer[3] -eq 0) {
        write-host "Debug: got ymx win update"
        Write-Host $ymxbuffer
     }
     elseif ($ymxbuffer[1] -eq 1 -and $ymxbuffer[3] -eq 4) {
        write-host "Debug: got ymx FIN"
	    $ymxstream = [bitconverter]::ToInt32($ymxbuffer[7..4],0)
	    write-host "Yamux stream ID: $ymxstream"

        #search in streamlist inpsteam and outstream by ymx Id and get asyncjobresult
        if ($streams.Count -gt 1){$streamind = $streams.ymxId.IndexOf($ymxstream)} 
        else {$streamind = 0}
        
        $AsyncJobResult = $streams[$streamind].asyncobj
        $PS = $streams[$streamind].psobj
        write-host "Debug: stoping thread"
        if ($StopFlag[$ymxstream] -eq 0){ 
            $StopFlag[$ymxstream] = 1 
        }
        start-sleep -milliseconds 200 #wait for thread check flag
        #$streams[$streamind].cinputStream.close()
        #$streams[$streamind].coutputStream.close()
        #$streams[$streamind].sinputStream.close()
        #$streams[$streamind].soutputStream.close()
        $streams[$streamind].psobj.Runspace.close()
        $streams[$streamind].psobj.Dispose()
        $streams[$streamind].readbuffer.clear()

        write-host "Debug: removing streams from streams list"
        $streams.RemoveAt($streamind)
        [System.GC]::Collect()#clear garbage to minimize memory usage

     
     }else{
	 	write-host "got something wrong"
        $wrnum += 1
        if ($wrnum -gt 10) { write-host "too many errors! Exiting..."; break;}
	 }

 }
 
	
 write-host "Debug: stoping ymx thread"
 $PS1.EndStop($PS1.BeginStop($null,$ymxAsyncJob))
 $tcpConnection.Close()
  
}else{
    write-host "not connected"
}

